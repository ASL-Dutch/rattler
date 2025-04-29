package component

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// FileHandleFunc 示例文件处理函数
func FileHandleFunc(filename string, additionalData interface{}) error {
	// 打印文件名和附加数据
	fmt.Printf("处理文件: %s，附加数据: %v\n", filename, additionalData)

	return nil
}

// StartXmlWatcher 启动XML文件监听服务示例（兼容原有功能）
func StartXmlWatcher(dir string, dc string) error {
	// 创建FSWatcher配置
	config := FSWatcherConfig{
		Dir:            dir,
		Operations:     Create,         // 只监听创建事件
		FilePattern:    ".*\\.xml",     // 只监听XML文件
		WaitTime:       5,              // 等待5秒
		MaxRetries:     10,             // 最多重试10次
		MinFileSize:    100,            // 最小文件大小100字节
		Handler:        FileHandleFunc, // 处理函数
		AdditionalData: dc,             // 传递申报国家信息
	}

	// 创建并启动监听服务
	watcher := NewFSWatcher(config)
	err := watcher.Start()
	if err != nil {
		return fmt.Errorf("启动XML文件监听服务失败: %v", err)
	}

	log.Infof("XML文件监听服务已启动，监听目录: %s，申报国家: %s", dir, dc)

	// 阻塞等待服务结束（实际使用时可能需要非阻塞方式）
	go watcher.WaitForCompletion()

	return nil
}

// Watch 兼容原有的Watch函数，内部使用新的FSWatcher实现
func Watch(dir string, dc string) {
	err := StartXmlWatcher(dir, dc)
	if err != nil {
		log.Error(err)
	}
}
