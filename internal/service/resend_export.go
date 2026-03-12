package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/util"
)

// moveOneExportFile 移动一个导出文件到监听目录以触发重新处理
// file: 源文件路径
// dc: 申报国家代码 (NL|BE)
// inListenDir: 文件是否已经在监听目录中
// customPath: 自定义路径
func moveOneExportFile(filename string, dc string, inListenDir bool, customPath string) (err error) {
	// 获取监听目录
	exportWatchDir := config.GlobalConfig.GetExportWatchDir(dc)

	// 备份路径
	exportBackupDir := config.GlobalConfig.GetExportBackupDir(dc)

	// 获取文件所在目录
	var fileDir string
	switch {
	case inListenDir:
		fileDir = exportWatchDir
	case customPath != "":
		fileDir = customPath
	case util.IsDatePrefix(filename, "_", "200601"):
		prefix := strings.Split(filename, "_")[0]
		dateDir, err := util.DateToPathFormat(prefix, "200601", 2)
		if err != nil {
			return fmt.Errorf("无法确定文件目录: %s。请检查文件名前缀是否为yyyyMM格式", filename)
		}
		fileDir = filepath.Join(exportBackupDir, dateDir)
	default:
		return fmt.Errorf("无法确定文件目录: %s。请检查文件是否在监听目录、自定义路径中，或文件名前缀是否为yyyyMM格式", filename)
	}

	filePath := filepath.Join(fileDir, filename)

	// 验证文件存在
	if !util.IsExists(filePath) {
		return fmt.Errorf("文件不存在: %s", filePath)
	}

	// 获取临时目录配置
	tmpDir := config.GlobalConfig.GetTempDir()
	if tmpDir == "" {
		tmpDir = os.TempDir() // 使用系统临时目录作为后备
	}

	// 确保临时目录存在
	if !util.IsDir(tmpDir) {
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Errorf("无法创建临时目录 %s: %v", tmpDir, err)
		}
	}

	// 如果文件在监听目录中，需要先移动到临时目录再移回来以触发文件创建事件
	if inListenDir {
		// 创建临时目录
		tmpSubDir := filepath.Join(tmpDir, fmt.Sprintf("export_resend_%d", time.Now().UnixNano()))
		if err := os.MkdirAll(tmpSubDir, 0755); err != nil {
			return fmt.Errorf("无法创建临时子目录 %s: %v", tmpSubDir, err)
		}

		// 移动到临时目录
		tmpFile := filepath.Join(tmpSubDir, filename)
		config.PublishFileMover(model.FileMoverParam{
			SourceFile: filePath,
			MoveTo:     tmpFile,
		})

		// 添加移动回监听目录的任务
		log.Infof("添加文件移回监听目录的任务: %s -> %s", tmpFile, exportWatchDir)
		config.PublishFileMover(model.FileMoverParam{
			SourceFile: tmpFile,
			MoveTo:     exportWatchDir,
		})

	} else {
		// 直接添加移动到监听目录的任务
		log.Infof("添加文件移动到监听目录的任务: %s -> %s", filePath, exportWatchDir)
		config.PublishFileMover(model.FileMoverParam{
			SourceFile: filePath,
			MoveTo:     exportWatchDir,
		})
	}

	return nil
}

// ResendExportFile 重新发送Export XML文件
// files: 文件路径列表
// dc: 申报国家代码 (NL|BE)
// inWatchPath: 是否在监听路径中
// customPath: 自定义路径
func ResendExportFile(files []string, dc string, inWatchPath bool, customPath string) []string {
	var results []string

	for _, file := range files {
		if err := moveOneExportFile(file, dc, inWatchPath, customPath); err != nil {
			log.Errorf("重新发送文件 %s 失败: %v", file, err)
			results = append(results, fmt.Sprintf("文件 %s 处理失败: %v", filepath.Base(file), err))
		} else {
			log.Infof("文件 %s 已成功加入重新发送队列", file)
			results = append(results, fmt.Sprintf("文件 %s 已加入重新发送队列", filepath.Base(file)))
		}
	}

	return results
}
