package component

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// FileOperation 定义文件操作类型
type FileOperation uint

// 支持的文件操作类型
const (
	Create FileOperation = 1 << iota
	Write
	Remove
	Rename
	Chmod
	All = Create | Write | Remove | Rename | Chmod
)

// HandlerFunc 文件处理函数类型
type HandlerFunc func(filename string, additionalData interface{}) error

// FSWatcherConfig 文件监听配置
type FSWatcherConfig struct {
	// Dir 监听目录
	Dir string
	// Operations 监听的文件操作类型，如 Create|Write
	Operations FileOperation
	// FilePattern 文件匹配模式（正则表达式），为空则匹配所有文件
	FilePattern string
	// WaitTime 处理延迟时间（秒），等待文件写入完成
	WaitTime int
	// MaxRetries 最大重试次数，检查文件是否可读
	MaxRetries int
	// MinFileSize 文件最小大小（字节），低于此大小将等待
	MinFileSize int64
	// Handler 文件处理函数
	Handler HandlerFunc
	// AdditionalData 传递给Handler的额外数据
	AdditionalData interface{}
}

// FSWatcher 文件监听服务
type FSWatcher struct {
	Config    FSWatcherConfig
	watcher   *fsnotify.Watcher
	done      chan bool
	isRunning bool
}

// NewFSWatcher 创建新的文件监听服务
func NewFSWatcher(config FSWatcherConfig) *FSWatcher {
	// 设置默认值
	if config.MaxRetries <= 0 {
		config.MaxRetries = 10
	}
	if config.WaitTime <= 0 {
		config.WaitTime = 5
	}
	if config.MinFileSize <= 0 {
		config.MinFileSize = 100
	}
	if config.Operations == 0 {
		config.Operations = Create
	}

	return &FSWatcher{
		Config: config,
		done:   make(chan bool),
	}
}

// Start 启动文件监听
func (fw *FSWatcher) Start() error {
	if fw.isRunning {
		return nil
	}

	var err error
	fw.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	fw.isRunning = true
	go fw.watchFiles()

	// 添加监听目录
	err = fw.watcher.Add(fw.Config.Dir)
	if err != nil {
		fw.Stop()
		return err
	}

	// 递归添加子目录
	err = filepath.Walk(fw.Config.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fw.watcher.Add(path)
		}
		return nil
	})

	if err != nil {
		fw.Stop()
		return err
	}

	log.Infof("开始监听目录: %s", fw.Config.Dir)
	return nil
}

// Stop 停止文件监听
func (fw *FSWatcher) Stop() {
	if !fw.isRunning {
		return
	}

	fw.isRunning = false
	if fw.watcher != nil {
		fw.watcher.Close()
	}
	fw.done <- true
	close(fw.done)
	log.Info("文件监听服务已停止")
}

// watchFiles 监听文件变化
func (fw *FSWatcher) watchFiles() {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("文件监听服务异常: %v", r)
		}
	}()

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			fw.handleFileEvent(event)
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Errorf("文件监听错误: %v", err)
		case <-fw.done:
			return
		}
	}
}

// handleFileEvent 处理文件事件
func (fw *FSWatcher) handleFileEvent(event fsnotify.Event) {
	log.Debugf("文件事件: %s %s", event.Name, event.Op)

	// 检查是否需要处理这种操作类型
	if !fw.shouldHandleOperation(event.Op) {
		return
	}

	// 如果是目录，添加到监听列表（仅对创建事件）
	if event.Op&fsnotify.Create == fsnotify.Create {
		fileInfo, err := os.Stat(event.Name)
		if err == nil && fileInfo.IsDir() {
			err = fw.watcher.Add(event.Name)
			if err != nil {
				log.Errorf("添加新目录到监听列表失败: %v", err)
			}
			log.Debugf("添加新目录到监听列表: %s", event.Name)
			return
		}
	}

	// 检查文件是否匹配模式
	if !fw.matchesFilePattern(event.Name) {
		return
	}

	// 等待文件写入完成
	filename := event.Name
	if fw.Config.WaitTime > 0 {
		canRead, _ := fw.waitFileWriteFinish(filename)
		if !canRead {
			log.Warnf("文件 %s 无法读取，跳过处理", filename)
			return
		}
	}

	// 调用处理函数
	if fw.Config.Handler != nil {
		go func() {
			err := fw.Config.Handler(filename, fw.Config.AdditionalData)
			if err != nil {
				log.Errorf("处理文件 %s 时出错: %v", filename, err)
			}
		}()
	}
}

// shouldHandleOperation 判断是否需要处理此类操作
func (fw *FSWatcher) shouldHandleOperation(op fsnotify.Op) bool {
	var operation FileOperation

	switch {
	case op&fsnotify.Create == fsnotify.Create:
		operation = Create
	case op&fsnotify.Write == fsnotify.Write:
		operation = Write
	case op&fsnotify.Remove == fsnotify.Remove:
		operation = Remove
	case op&fsnotify.Rename == fsnotify.Rename:
		operation = Rename
	case op&fsnotify.Chmod == fsnotify.Chmod:
		operation = Chmod
	}

	return fw.Config.Operations&operation != 0
}

// matchesFilePattern 判断文件是否匹配模式
func (fw *FSWatcher) matchesFilePattern(filename string) bool {
	if fw.Config.FilePattern == "" {
		return true
	}

	compile, err := regexp.Compile(fw.Config.FilePattern)
	if err != nil {
		log.Errorf("文件模式正则表达式编译错误: %v", err)
		return false
	}

	return compile.MatchString(filename)
}

// waitFileWriteFinish 等待文件写入完成
func (fw *FSWatcher) waitFileWriteFinish(filename string) (bool, error) {
	var retries = fw.Config.MaxRetries
	var waitTime = time.Duration(fw.Config.WaitTime) * time.Second
	var minSize = fw.Config.MinFileSize

	for retries > 0 {
		retries--

		fileInfo, err := os.Stat(filename)
		if err != nil {
			if retries == 0 {
				log.Errorf("文件 %s 无法访问: %v", filename, err)
				return false, err
			}
			log.Debugf("文件 %s 可能仍在写入中，等待 %d 秒后重试...", filename, fw.Config.WaitTime)
			time.Sleep(waitTime)
			continue
		}

		// 检查文件大小
		if fileInfo.Size() < minSize {
			if retries == 0 {
				log.Warnf("文件 %s 大小(%d)小于最小要求(%d)，但已达到最大重试次数",
					filename, fileInfo.Size(), minSize)
				return true, nil
			}
			log.Debugf("文件 %s 大小(%d)小于最小要求(%d)，等待 %d 秒后重试...",
				filename, fileInfo.Size(), minSize, fw.Config.WaitTime)
			time.Sleep(waitTime)
			continue
		}

		// 尝试打开文件验证是否可读
		file, err := os.Open(filename)
		if err != nil {
			if retries == 0 {
				log.Errorf("文件 %s 无法打开: %v", filename, err)
				return false, err
			}
			log.Debugf("文件 %s 可能被锁定，等待 %d 秒后重试...", filename, fw.Config.WaitTime)
			time.Sleep(waitTime)
			continue
		}
		file.Close()

		// 文件可以打开且大小合适，认为写入完成
		return true, nil
	}

	return false, nil
}

// WaitForCompletion 等待文件监听完成（阻塞方法）
func (fw *FSWatcher) WaitForCompletion() {
	<-fw.done
}
