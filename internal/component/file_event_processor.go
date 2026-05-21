package component

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// FileEvent 文件事件消息结构
type FileEvent struct {
	FilePath       string      // 文件完整路径
	DetectedTime   time.Time   // 检测到的时间
	AdditionalData interface{} // 额外数据（如国家代码）
}

// FileProcessorConfig 文件处理器配置（单队列单消费者）
type FileProcessorConfig struct {
	// EventChannel 事件消息通道（只读），统一队列
	EventChannel <-chan FileEvent

	// Handler 文件处理函数（读取并发送）
	Handler HandlerFunc

	// JobNoExtractor 从文件名提取 job no，仅用于日志，不参与路由
	JobNoExtractor func(filePath string) (string, error)

	// WaitTime 已废弃：请使用 PollIntervalMs；若 PollIntervalMs 未设置则按秒换算
	WaitTime int

	// MaxRetries 最大轮询次数（写入完成检测）
	MaxRetries int

	// PollIntervalMs 每次 stat 间隔（毫秒）；未设置时由 WaitTime（秒）换算
	PollIntervalMs int

	// StabilityRequiredHits 连续多少次文件大小相同视为写入稳定（默认 2）
	StabilityRequiredHits int

	// MinFileSize 文件最小大小（字节）
	MinFileSize int64
}

// FileProcessor 文件处理器：从统一队列按 FIFO 顺序同步消费（等可读 → 读 → 发）
type FileProcessor struct {
	config   FileProcessorConfig
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewFileProcessor 创建新的文件处理器（单消费者，严格按队列顺序处理）
func NewFileProcessor(config FileProcessorConfig) *FileProcessor {
	if config.WaitTime <= 0 {
		config.WaitTime = 5
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 40
	}
	if config.PollIntervalMs <= 0 {
		config.PollIntervalMs = config.WaitTime * 1000
		if config.PollIntervalMs <= 0 {
			config.PollIntervalMs = 500
		}
	}
	if config.StabilityRequiredHits <= 0 {
		config.StabilityRequiredHits = 2
	}
	if config.MinFileSize <= 0 {
		config.MinFileSize = 100
	}
	if config.JobNoExtractor == nil {
		config.JobNoExtractor = ExtractBusinessKeyFromFileName
	}
	return &FileProcessor{
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// Start 启动文件处理器：单 goroutine 从统一队列按序消费
func (fp *FileProcessor) Start() {
	log.Info("文件处理器启动：单队列按创建顺序消费（等写入稳定 → 读 → 发）")
	fp.wg.Add(1)
	go fp.consumeLoop()
}

// Stop 停止文件处理器
func (fp *FileProcessor) Stop() {
	log.Info("文件处理器正在停止...")
	close(fp.stopChan)
	fp.wg.Wait()
	log.Info("文件处理器已停止")
}

// consumeLoop 单消费者：从 EventChannel 严格按 FIFO 取事件，同步等待可读后执行 Handler
func (fp *FileProcessor) consumeLoop() {
	defer fp.wg.Done()
	for {
		select {
		case <-fp.stopChan:
			return
		case event, ok := <-fp.config.EventChannel:
			if !ok {
				log.Info("事件通道已关闭，文件处理器退出")
				return
			}
			fp.processOne(event)
		}
	}
}

// processOne 处理单条事件：同步等待文件可读后调用 Handler（保证同业务多文件按创建顺序）
func (fp *FileProcessor) processOne(event FileEvent) {
	jobNo := fp.logJobNo(event.FilePath)

	log.Debugf("从队列取出文件事件: %s (jobNo=%s)", filepath.Base(event.FilePath), jobNo)

	canRead, err := fp.waitFileWriteFinish(event.FilePath)
	if !canRead {
		log.Warnf("文件未就绪，跳过处理: %s, err=%v", event.FilePath, err)
		return
	}

	log.Debugf("文件已就绪，开始业务处理: %s (jobNo=%s)", event.FilePath, jobNo)

	if fp.config.Handler != nil {
		if err := fp.config.Handler(event.FilePath, event.AdditionalData); err != nil {
			log.Errorf("处理文件失败 [jobNo=%s]: %s, err=%v", jobNo, event.FilePath, err)
		} else {
			log.Infof("处理文件完成 [jobNo=%s]: %s", jobNo, filepath.Base(event.FilePath))
		}
	}
}

func (fp *FileProcessor) logJobNo(filePath string) string {
	if fp.config.JobNoExtractor == nil {
		return "-"
	}
	jobNo, err := fp.config.JobNoExtractor(filePath)
	if err != nil {
		return "-"
	}
	return jobNo
}

// waitFileWriteFinish 等待文件创建且内容写入完成：存在、达到最小大小、连续多次大小一致、可打开。
func (fp *FileProcessor) waitFileWriteFinish(filename string) (bool, error) {
	opts := fileStabilityOptions{
		maxAttempts:           fp.config.MaxRetries,
		pollInterval:          time.Duration(fp.config.PollIntervalMs) * time.Millisecond,
		stabilityRequiredHits: fp.config.StabilityRequiredHits,
		minSize:               fp.config.MinFileSize,
	}
	return waitForFileStable(filename, opts)
}

type fileStabilityOptions struct {
	maxAttempts           int
	pollInterval          time.Duration
	stabilityRequiredHits int
	minSize               int64
}

// waitForFileStable 轮询直到文件大小连续稳定且可打开，或超出尝试次数。
func waitForFileStable(filename string, opts fileStabilityOptions) (bool, error) {
	if opts.maxAttempts <= 0 {
		opts.maxAttempts = 40
	}
	if opts.pollInterval <= 0 {
		opts.pollInterval = 500 * time.Millisecond
	}
	if opts.stabilityRequiredHits <= 0 {
		opts.stabilityRequiredHits = 2
	}

	var (
		lastSize   int64 = -1
		stableHits int
		lastErr    error
	)

	for attempt := 1; attempt <= opts.maxAttempts; attempt++ {
		info, err := os.Stat(filename)
		if err != nil {
			lastErr = err
			stableHits = 0
			lastSize = -1
			log.Debugf("文件可能仍在创建(%d/%d): %s, err=%v", attempt, opts.maxAttempts, filename, err)
			time.Sleep(opts.pollInterval)
			continue
		}

		size := info.Size()
		if size < opts.minSize {
			lastErr = fmt.Errorf("file too small: %d < %d", size, opts.minSize)
			stableHits = 0
			lastSize = size
			log.Debugf("文件大小不足(%d/%d): %s, size=%d", attempt, opts.maxAttempts, filename, size)
			time.Sleep(opts.pollInterval)
			continue
		}

		if size == lastSize {
			stableHits++
		} else {
			stableHits = 1
			lastSize = size
		}

		if stableHits < opts.stabilityRequiredHits {
			log.Debugf("文件大小尚未稳定(%d/%d): %s, size=%d, stable=%d/%d",
				attempt, opts.maxAttempts, filename, size, stableHits, opts.stabilityRequiredHits)
			time.Sleep(opts.pollInterval)
			continue
		}

		f, err := os.Open(filename)
		if err != nil {
			lastErr = err
			stableHits = 0
			log.Debugf("文件可能被锁定(%d/%d): %s, err=%v", attempt, opts.maxAttempts, filename, err)
			time.Sleep(opts.pollInterval)
			continue
		}
		_ = f.Close()
		log.Debugf("文件写入已稳定(%d/%d): %s, size=%d", attempt, opts.maxAttempts, filename, size)
		return true, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("file not stable after %d attempts", opts.maxAttempts)
	}
	return false, lastErr
}

// ExtractBusinessKeyFromFileName 从文件名提取 job no（用于日志等）
// 文件名格式示例: 26484_NI-2025-886_09.xml -> job no 886
func ExtractBusinessKeyFromFileName(filePath string) (string, error) {
	filename := filepath.Base(filePath)
	ext := filepath.Ext(filename)
	nameWithoutExt := filename
	if ext != "" {
		nameWithoutExt = filename[:len(filename)-len(ext)]
	}

	parts := strings.Split(nameWithoutExt, "_")
	for _, part := range parts {
		if !strings.Contains(part, "-") {
			continue
		}
		segments := strings.Split(part, "-")
		if len(segments) < 2 {
			continue
		}
		jobNo := strings.TrimSpace(segments[len(segments)-1])
		if jobNo != "" {
			return jobNo, nil
		}
	}
	return nameWithoutExt, nil
}

// ExtractBusinessKeyFromFilePath 使用文件路径作为键（兼容）
func ExtractBusinessKeyFromFilePath(filePath string) (string, error) {
	return filePath, nil
}
