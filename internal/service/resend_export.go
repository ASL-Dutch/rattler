package service

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sysafari.com/softpak/rattler/internal/component"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/util"
)

// moveOneExportFile 移动一个导出文件到监听目录以触发重新处理
// file: 源文件路径
// dc: 申报国家代码 (NL|BE)
// inListenDir: 文件是否已经在监听目录中
func moveOneExportFile(file string, dc string, inListenDir bool) (err error) {
	// 获取对应国家的监听目录和移动处理程序
	var exportWatchDir string
	var remover *component.RemoveQueue

	switch dc {
	case "NL":
		exportWatchDir = viper.GetString("watcher.nl.watch-dir")
		remover = config.NlRemover
	case "BE":
		exportWatchDir = viper.GetString("watcher.be.watch-dir")
		remover = config.BeRemover
	default:
		return fmt.Errorf("无效的申报国家代码: %s", dc)
	}

	// 验证监听目录配置
	if exportWatchDir == "" {
		return fmt.Errorf("申报国家 %s 的监听目录未配置", dc)
	}

	// 验证文件存在
	if !util.IsExists(file) {
		return fmt.Errorf("文件不存在: %s", file)
	}

	// 获取临时目录配置
	tmpDir := viper.GetString("tmp-dir")
	if tmpDir == "" {
		tmpDir = os.TempDir() // 使用系统临时目录作为后备
	}

	// 确保临时目录存在
	if !util.IsDir(tmpDir) {
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			return fmt.Errorf("无法创建临时目录 %s: %v", tmpDir, err)
		}
	}

	// 获取文件名
	fn := filepath.Base(file)

	// 如果文件在监听目录中，需要先移动到临时目录再移回来以触发文件创建事件
	if inListenDir {
		fileTmp := filepath.Join(tmpDir, fn)
		log.Infof("[%s] 临时移动文件: %s -> %s", dc, file, fileTmp)

		if err = os.Rename(file, fileTmp); err != nil {
			return fmt.Errorf("移动文件到临时目录失败: %v", err)
		}

		// 等待文件系统操作完成
		time.Sleep(1 * time.Second)
		file = fileTmp
	}

	// 将文件添加到移动队列
	log.Infof("[%s] 添加文件到移动队列: %s -> %s", dc, file, exportWatchDir)
	remover.Add(component.RemoveParam{
		SourceFile: file,
		MoveTo:     exportWatchDir,
	})

	return nil
}

// ResendExport 重新发送Export文件
// dc: 申报国家代码 (NL|BE)
// params: 包含文件路径和监听状态的请求参数
func ResendExport(dc string, params *model.FileResendRequest) (errs []string) {
	if params == nil || len(params.FilePaths) == 0 {
		return []string{"没有指定要重发的文件"}
	}

	log.Infof("[%s] 开始重发 %d 个文件", dc, len(params.FilePaths))

	for _, path := range params.FilePaths {
		if err := moveOneExportFile(path, dc, params.InListeningPath); err != nil {
			log.Errorf("[%s] 重发文件失败: %s: %v", dc, path, err)
			errs = append(errs, err.Error())
		} else {
			log.Infof("[%s] 文件已加入重发队列: %s", dc, path)
		}
	}

	if len(errs) > 0 {
		log.Warnf("[%s] 重发过程中有 %d 个错误", dc, len(errs))
	} else {
		log.Infof("[%s] 所有文件已成功加入重发队列", dc)
	}

	return errs
}
