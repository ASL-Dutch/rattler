package service

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/util"
)

// TaxBillService 税金单文件处理服务
type TaxBillService struct{}

// NewTaxBillService 创建税金单服务实例
func NewTaxBillService() *TaxBillService {
	return &TaxBillService{}
}

// MoveTaxBillToBackup 将税金单文件移动到备份目录
// 备份目录结构为: backupDir/yyyy/mm/
// 文件名格式为: yyyymm_originalFileName
func (s *TaxBillService) MoveTaxBillToBackup(filePath, country string) (string, error) {
	// 检查文件是否存在
	if !util.IsExists(filePath) {
		err := fmt.Errorf("源文件不存在: %s", filePath)
		log.Error(err)
		return "", err
	}

	// 获取文件名
	fileName := filepath.Base(filePath)

	// 获取当前时间，用于创建目录结构和文件名前缀
	now := time.Now()
	year := now.Format("2006")
	month := now.Format("01")
	prefix := fmt.Sprintf("%s%s_", year, month)

	// 检查文件名是否已有前缀，如果有则不再添加
	if !strings.HasPrefix(fileName, prefix) {
		fileName = prefix + fileName
	}

	// 从配置获取备份目录
	backupDir := config.GlobalConfig.GetTaxBillDir(country)
	if backupDir == "" {
		err := fmt.Errorf("国家 %s 的税金单备份目录未配置", country)
		log.Error(err)
		return "", err
	}

	// 构建目标目录路径: backupDir/yyyy/mm/
	targetDir := filepath.Join(backupDir, year, month)
	// 将文件后缀统一为小写的pdf
	fileNameWithoutExt := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	targetPath := filepath.Join(targetDir, fileNameWithoutExt+".pdf")

	log.Infof("准备将税金单文件 %s 移动到 %s", filePath, targetPath)

	// 使用异步文件移动服务
	fileMoverParam := model.FileMoverParam{
		SourceFile: filePath,
		MoveTo:     targetPath,
		// 如果启用冗余备份，则使用复制，否则使用移动
		IsCopy:     config.GlobalConfig.IsKeepOriginalEnabled(country),
	}

	// 通过消息队列发布文件移动请求
	config.PublishFileMover(fileMoverParam)

	log.Infof("税金单文件 %s 已提交移动请求", fileName)

	return fileName, nil
}

// taxBillLookupRoots 税金单 Web 访问目录（storage.tax-bill-original / tax-bill-backup，不引用 watchers 监听路径）。
type taxBillLookupRoots struct {
	original string
	backup   string
}

type taxBillLookupAttempt struct {
	dir        string
	lookupName string
	note       string
	found      bool
	resolved   string
}

type taxBillDownloadLookupTrace struct {
	country     string
	requestFile string
	originalDir string
	backupDir   string
	attempts    []taxBillLookupAttempt
}

func (t *taxBillDownloadLookupTrace) addAttempt(dir, lookupName, note string) *taxBillLookupAttempt {
	att := taxBillLookupAttempt{dir: dir, lookupName: lookupName, note: note}
	t.attempts = append(t.attempts, att)
	return &t.attempts[len(t.attempts)-1]
}

func logTaxBillDownloadLookup(trace taxBillDownloadLookupTrace, resolvedPath string, findErr error) {
	var parts []string
	for _, a := range trace.attempts {
		status := "未命中"
		if a.found {
			status = "命中->" + a.resolved
		}
		dir := a.dir
		if dir == "" {
			dir = "(未配置)"
		}
		part := fmt.Sprintf("dir=%s lookup=%s %s", dir, a.lookupName, status)
		if a.note != "" {
			part += " (" + a.note + ")"
		}
		parts = append(parts, part)
	}
	attempts := strings.Join(parts, "; ")
	if findErr != nil {
		log.Warnf(
			"税金单下载查找失败: country=%s file=%s originalDir=%s backupDir=%s resolved= attempts=[%s] err=%v",
			trace.country, trace.requestFile, trace.originalDir, trace.backupDir, attempts, findErr,
		)
		return
	}
	log.Infof(
		"税金单下载查找成功: country=%s file=%s originalDir=%s backupDir=%s resolved=%s attempts=[%s]",
		trace.country, trace.requestFile, trace.originalDir, trace.backupDir, resolvedPath, attempts,
	)
}

// FindTaxBillFile 按文件名解析税金单物理路径（仅使用 storage.tax-bill-original / tax-bill-backup）。
//
// 规则：
//   - 含 yyyyMM_ 前缀：优先 backup/yyyy/mm/ 下全名；再在 original 下去前缀查找（未备份历史文件）。
//   - 无前缀：优先 original；再在 backup 树中匹配 yyyyMM_原始名。
func (s *TaxBillService) FindTaxBillFile(filename, country string) (string, error) {
	if filename == "" || country == "" {
		return "", fmt.Errorf("文件名和国家代码不能为空")
	}
	filename = util.NormalizeTaxBillFileName(filename)

	roots, err := s.resolveTaxBillLookupRoots(country)
	if err != nil {
		return "", err
	}

	trace := taxBillDownloadLookupTrace{
		country:     country,
		requestFile: filename,
		originalDir: roots.original,
		backupDir:   roots.backup,
	}

	var resolved string
	tryLookup := func(lookupName, dir, note string) bool {
		if lookupName == "" {
			return false
		}
		att := trace.addAttempt(dir, lookupName, note)
		if dir == "" {
			return false
		}
		path, err := s.findFileInDirectory(lookupName, dir)
		if err != nil {
			return false
		}
		att.found = true
		att.resolved = path
		resolved = path
		return true
	}

	if year, month, ok := util.ParseTaxBillBackupPrefix(filename); ok {
		unprefixed := util.TaxBillNameWithoutBackupPrefix(filename)
		lookups := []struct {
			name string
			dir  string
			note string
		}{
			{filename, filepath.Join(roots.backup, year, month), "backup-dated"},
			{unprefixed, roots.original, "original-unprefixed"},
			{filename, roots.original, "original-prefixed"},
			{filename, roots.backup, "backup-root"},
		}
		for _, item := range lookups {
			if tryLookup(item.name, item.dir, item.note) {
				logTaxBillDownloadLookup(trace, resolved, nil)
				return resolved, nil
			}
		}
	} else {
		if tryLookup(filename, roots.original, "original") {
			logTaxBillDownloadLookup(trace, resolved, nil)
			return resolved, nil
		}
		att := trace.addAttempt(roots.backup, filename, "backup-tree-glob")
		if path, err := s.findBackedUpCopyByOriginalName(roots.backup, filename); err == nil {
			att.found = true
			att.resolved = path
			resolved = path
			logTaxBillDownloadLookup(trace, resolved, nil)
			return resolved, nil
		}
		if tryLookup(filename, roots.backup, "backup-root") {
			logTaxBillDownloadLookup(trace, resolved, nil)
			return resolved, nil
		}
	}

	findErr := fmt.Errorf("未找到税金单文件 %s", filename)
	logTaxBillDownloadLookup(trace, "", findErr)
	return "", findErr
}

func (s *TaxBillService) resolveTaxBillLookupRoots(country string) (taxBillLookupRoots, error) {
	roots := taxBillLookupRoots{
		original: strings.TrimSpace(config.GlobalConfig.GetStorageTaxBillOriginalDir(country)),
		backup:   strings.TrimSpace(config.GlobalConfig.GetStorageTaxBillBackupDir(country)),
	}
	if roots.original == "" && roots.backup == "" {
		return roots, fmt.Errorf("国家 %s 的税金单存储目录未配置（storage.tax-bill-original / storage.tax-bill-backup）", country)
	}
	return roots, nil
}

// findBackedUpCopyByOriginalName 在 backup/yyyy/mm/ 下查找 yyyyMM_原始文件名（监听备份后 watch 可能已无原文件）。
func (s *TaxBillService) findBackedUpCopyByOriginalName(backupDir, originalName string) (string, error) {
	if backupDir == "" {
		return "", fmt.Errorf("backup dir empty")
	}
	pattern := filepath.Join(backupDir, "20*", "*", "*_"+originalName)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	var candidates []string
	suffix := "_" + originalName
	for _, m := range matches {
		base := filepath.Base(m)
		if strings.EqualFold(base, originalName) {
			candidates = append(candidates, m)
			continue
		}
		if strings.HasSuffix(strings.ToLower(base), strings.ToLower(suffix)) {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no backed-up copy for %s", originalName)
	}
	sort.Strings(candidates)
	return candidates[len(candidates)-1], nil
}

func uniqueNonEmptyDirs(dirs ...string) []string {
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

// findFileInDirectory 在指定目录中查找文件
// 首先尝试精确匹配，然后尝试不区分大小写匹配
func (s *TaxBillService) findFileInDirectory(filename, dirPath string) (string, error) {
	// 构建完整文件路径
	filePath := filepath.Join(dirPath, filename)
	log.Debugf("在目录 %s 中查找文件: %s", dirPath, filename)

	// 检查文件是否存在（精确匹配）
	if util.IsExists(filePath) {
		return filePath, nil
	}

	// 尝试不区分大小写查找
	pdfFiles, err := filepath.Glob(filepath.Join(dirPath, "*.pdf"))
	if err != nil {
		log.Warnf("获取目录 %s 下的PDF文件失败: %v", dirPath, err)
		return "", fmt.Errorf("搜索文件时发生错误: %v", err)
	}

	// 遍历所有PDF文件，进行不区分大小写的比较
	for _, file := range pdfFiles {
		if strings.EqualFold(filepath.Base(file), filename) {
			return file, nil
		}
	}

	// 文件未找到
	return "", fmt.Errorf("在目录 %s 中未找到文件 %s", dirPath, filename)
}
