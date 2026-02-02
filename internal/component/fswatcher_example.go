package component

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// FileHandleFunc 示例文件处理函数
func FileHandleFunc(filename string, additionalData interface{}) error {
	fmt.Printf("处理文件: %s，附加数据: %v\n", filename, additionalData)
	return nil
}

// StartXmlWatcher 启动 XML 文件监听服务示例（事件通道 + 单消费者）
func StartXmlWatcher(dir string, dc string) error {
	eventChannel := make(chan FileEvent, EventChannelBaseSize+EventChannelExtraSize)

	processorConfig := FileProcessorConfig{
		EventChannel:   eventChannel,
		Handler:        FileHandleFunc,
		JobNoExtractor: ExtractBusinessKeyFromFileName,
		WaitTime:       5,
		MaxRetries:     10,
		MinFileSize:    100,
	}
	processor := NewFileProcessor(processorConfig)
	processor.Start()

	watcherConfig := FSWatcherConfig{
		Dir:            dir,
		Operations:     Create,
		FilePattern:    `.*\.xml`,
		EventChannel:   eventChannel,
		AdditionalData: dc,
	}
	watcher := NewFSWatcher(watcherConfig)
	if err := watcher.Start(); err != nil {
		return fmt.Errorf("启动XML文件监听服务失败: %w", err)
	}

	log.Infof("XML文件监听服务已启动，监听目录: %s，申报国家: %s", dir, dc)
	go watcher.WaitForCompletion()
	return nil
}

// Watch 兼容原有 Watch 入口
func Watch(dir string, dc string) {
	if err := StartXmlWatcher(dir, dc); err != nil {
		log.Error(err)
	}
}
