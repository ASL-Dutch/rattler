package service

import (
	"fmt"
	"path/filepath"
	"regexp"
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

// FindTaxBillFile 查找税金单文件
// 如果文件名有yyyyMM_前缀，则在备份路径/yyyy/MM/目录下查找
// 如果没有前缀，则在备份路径根目录下查找，但不遍历子目录
func (s *TaxBillService) FindTaxBillFile(filename, country string) (string, error) {
	// 检查参数
	if filename == "" || country == "" {
		return "", fmt.Errorf("文件名和国家代码不能为空")
	}

	// 获取税金单备份目录
	backupDir := config.GlobalConfig.GetTaxBillDir(country)
	if backupDir == "" {
		return "", fmt.Errorf("国家 %s 的税金单备份目录未配置", country)
	}

	// 检查文件名是否有yyyyMM_前缀（使用正则表达式更精确地匹配）
	prefixRegex := regexp.MustCompile(`^(20\d{2})(0[1-9]|1[0-2])_`)
	matches := prefixRegex.FindStringSubmatch(filename)

	// 根据是否有前缀决定查找位置
	if len(matches) > 0 {
		// 提取年份和月份
		year := matches[1]  // 第一个捕获组 (20\d{2})
		month := matches[2] // 第二个捕获组 (0[1-9]|1[0-2])

		return s.findFileInDirectory(filename, filepath.Join(backupDir, year, month))
	} else {
		// 没有前缀，在监听路径下查找
		watchDir := config.GlobalConfig.GetPdfWatchDir(country)
		if watchDir == "" {
			return "", fmt.Errorf("国家 %s 的税金单监听目录未配置", country)
		}

		if !util.IsExists(watchDir) {
			return "", fmt.Errorf("国家 %s 的税金单监听目录不存在", country)
		}

		return s.findFileInDirectory(filename, watchDir)
	}
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
