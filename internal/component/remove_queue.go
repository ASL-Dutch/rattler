package component

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
)

// RemoveParam 定义文件移动操作的参数
type RemoveParam struct {
	// SourceFile 源文件路径
	SourceFile string

	// MoveTo 目标目录
	MoveTo string
}

// RemoveQueue 文件移动队列，基于 channel 实现
type RemoveQueue struct {
	// Queue 文件移动队列
	Queue chan RemoveParam

	// Name 队列名称，用于日志标识
	Name string
}

// NewRemoveQueue 创建并初始化一个新的移动队列
// size: 队列的缓冲大小
// name: 队列的名称标识（如 "NL", "BE"）
func NewRemoveQueue(size int, name string) *RemoveQueue {
	log.Infof("初始化 %s 文件移动队列，缓冲大小: %d", name, size)
	return &RemoveQueue{
		Queue: make(chan RemoveParam, size),
		Name:  name,
	}
}

// Add 添加文件移动任务到队列
func (q *RemoveQueue) Add(param RemoveParam) {
	log.Debugf("[%s] 添加移动任务: %s -> %s", q.Name, param.SourceFile, param.MoveTo)
	q.Queue <- param
}

// Run 启动文件移动处理循环
func (q *RemoveQueue) Run() {
	log.Infof("[%s] 启动文件移动处理程序", q.Name)
	defer func() {
		log.Warnf("[%s] 文件移动处理程序关闭", q.Name)
		close(q.Queue)
	}()

	for param := range q.Queue {
		srcFile := param.SourceFile
		filename := filepath.Base(srcFile)
		targetPath := filepath.Join(param.MoveTo, filename)

		// 确保目标目录存在
		if err := os.MkdirAll(param.MoveTo, 0755); err != nil {
			log.Errorf("[%s] 无法创建目标目录 %s: %v", q.Name, param.MoveTo, err)
			continue
		}

		log.Infof("[%s] 移动文件: %s 到目录: %s", q.Name, srcFile, targetPath)
		err := os.Rename(srcFile, targetPath)
		if err != nil {
			log.Errorf("[%s] 移动文件失败: %v", q.Name, err)
		} else {
			log.Infof("[%s] 成功移动文件: %s -> %s", q.Name, srcFile, targetPath)
		}
	}
}
