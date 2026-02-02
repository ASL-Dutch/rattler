package component

import (
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

const (
	// EventChannelBaseSize 事件通道基础容量
	EventChannelBaseSize = 1000
	// EventChannelExtraSize 通道满时临时增加的容量（总容量 = Base + Extra）
	EventChannelExtraSize = 500
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

// FSWatcherConfig 文件监听配置（仅负责监听并入队，不处理文件）
type FSWatcherConfig struct {
	// Dir 监听目录
	Dir string
	// Operations 监听的文件操作类型，如 Create|Write
	Operations FileOperation
	// FilePattern 文件匹配模式（正则表达式），为空则匹配所有文件
	FilePattern string
	// AdditionalData 随事件传递的额外数据（如国家代码）
	AdditionalData interface{}
	// EventChannel 事件消息通道，监听到的文件事件写入此通道，由 FileProcessor 消费
	EventChannel chan<- FileEvent
	// WatchSubdirs 是否递归监听子目录；默认 false，仅监听目标目录本身，避免递归风险
	WatchSubdirs bool
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

	// 设置监听服务运行状态
	fw.isRunning = true
	// 启动文件监听协程
	go fw.watchFiles()

	// 添加监听目录
	err = fw.watcher.Add(fw.Config.Dir)
	if err != nil {
		fw.Stop()
		return err
	}

	if fw.Config.WatchSubdirs {
		err = filepath.Walk(fw.Config.Dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && path != fw.Config.Dir {
				return fw.watcher.Add(path)
			}
			return nil
		})
		if err != nil {
			fw.Stop()
			return err
		}
		log.Infof("开始监听目录（含子目录）: %s", fw.Config.Dir)
	} else {
		log.Infof("开始监听目录（仅当前目录，不递归子目录）: %s", fw.Config.Dir)
	}
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

	// 如果是目录且开启了递归监听，将新创建的子目录加入监听（仅对创建事件）
	if fw.Config.WatchSubdirs && event.Op&fsnotify.Create == fsnotify.Create {
		fileInfo, err := os.Stat(event.Name)
		if err == nil && fileInfo.IsDir() {
			err = fw.watcher.Add(event.Name)
			if err != nil {
				log.Errorf("添加新目录到监听列表失败: %v", err)
			} else {
				log.Debugf("添加新子目录到监听列表: %s", event.Name)
			}
			return
		}
	}

	// 检查文件是否匹配模式
	if !fw.matchesFilePattern(event.Name) {
		return
	}

	// 仅入队：发送事件到统一队列，由 FileProcessor 按序消费（阻塞写入，不丢事件）
	if fw.Config.EventChannel == nil {
		log.Debugf("EventChannel 未设置，忽略文件事件: %s", filepath.Base(event.Name))
		return
	}
	fileEvent := FileEvent{
		FilePath:       event.Name,
		DetectedTime:   time.Now(),
		AdditionalData: fw.Config.AdditionalData,
	}
	fw.Config.EventChannel <- fileEvent
	log.Debugf("文件事件已入队: %s", filepath.Base(event.Name))
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

// WaitForCompletion 等待文件监听完成（阻塞方法）
func (fw *FSWatcher) WaitForCompletion() {
	<-fw.done
}
