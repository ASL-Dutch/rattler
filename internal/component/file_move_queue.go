package component

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// ProcessFunc 自定义处理函数类型
type ProcessFunc func(param string) error

// CustomQueue 自定义队列，基于 channel 实现
type CustomQueue struct {
	// Queue 文件移动队列
	Queue chan string

	// Name 队列名称，用于日志标识
	Name string

	// WorkerCount 并发工作进程数量
	WorkerCount int

	// WaitGroup 用于等待所有工作进程完成
	wg sync.WaitGroup

	// 标记队列是否已停止
	stopped bool
}

// NewCustomQueue 创建并初始化一个新的自定义队列
// size: 队列的缓冲大小
// name: 队列的名称标识（如 "global_async_file_mover"）
// workerCount: 并发工作进程数量
func NewCustomQueue(size int, name string, workerCount int) *CustomQueue {
	// 限制最大并发数为5
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > 5 {
		log.Warnf("并发处理线程最大不允许超过 5 个，当前设置为：5")
		workerCount = 5
	}

	log.Infof("初始化 %s 队列，缓冲大小: %d，并发数: %d", name, size, workerCount)
	return &CustomQueue{
		Queue:       make(chan string, size),
		Name:        name,
		WorkerCount: workerCount,
		stopped:     false,
	}
}

// Publish 发布消息到队列
func (q *CustomQueue) Publish(param string) {
	if q.stopped {
		log.Warnf("[%s] 队列已停止，无法添加新任务", q.Name)
		return
	}
	log.Debugf("[%s] 添加任务: %s", q.Name, param)
	q.Queue <- param
}

// worker 工作进程函数
func (q *CustomQueue) worker(idx int, handler ProcessFunc) {
	// 为每个worker生成唯一标识
	workerName := fmt.Sprintf("%s_%d", q.Name, idx)
	defer q.wg.Done()

	log.Infof("[%s] 工作进程启动", workerName)

	for param := range q.Queue {
		log.Debugf("[%s] 处理任务: %s", workerName, param)

		err := handler(param)

		if err != nil {
			log.Errorf("[%s] 处理任务失败: %v", workerName, err)
		} else {
			log.Debugf("[%s] 处理任务成功", workerName)
		}
	}

	log.Infof("[%s] 工作进程关闭", workerName)
}

// StartConsumer 启动消费者处理循环
// handler: 传递处理函数来异步执行
func (q *CustomQueue) StartConsumer(handler ProcessFunc) {
	if handler == nil {
		log.Errorf("[%s] 启动失败：未提供处理函数", q.Name)
		return
	}

	log.Infof("[%s] 启动处理程序，并发数: %d", q.Name, q.WorkerCount)

	// 启动指定数量的工作进程，每个进程都有唯一的索引
	for i := 0; i < q.WorkerCount; i++ {
		q.wg.Add(1)
		go q.worker(i+1, handler) // 传递索引和处理函数
	}
}

// Wait 等待所有工作进程完成
func (q *CustomQueue) Wait() {
	q.wg.Wait()
	log.Warnf("[%s] 所有处理程序已关闭", q.Name)
}

// StopConsumer 停止消费者处理循环
func (q *CustomQueue) StopConsumer() {
	if q.stopped {
		return
	}

	log.Infof("[%s] 正在停止队列", q.Name)
	q.stopped = true
	close(q.Queue)
}
