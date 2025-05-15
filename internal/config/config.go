// Package config provides configuration management for the application.
// It centralizes all configuration parameters and provides a unified interface
// for accessing them without direct coupling to viper.
package config

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sysafari.com/softpak/rattler/internal/component"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/util"
)

// Global configuration instance and removers
var (
	GlobalConfig *AppConfig

	// FileMoverQueue 全局文件移动队列
	FileMoverQueue *component.CustomQueue
)

// Apponfig represents the root application configuration
type AppConfig struct {
	// Port is the HTTP server port
	Port int `mapstructure:"port"`

	// Log contains logging configuration
	Log LogConfig `mapstructure:"log"`

	// Storage contains service directories for different countries
	Storage ServiceDirs `mapstructure:"storage"`

	// Import contains import configuration
	Import ImportConfig `mapstructure:"import"`

	// Watchers configuration for monitoring files
	Watchers WatchersConfig `mapstructure:"watchers"`

	// TempDir is the directory for temporary files
	TempDir string `mapstructure:"tmp-dir"`

	// FileMover configuration for file mover
	FileMover FileMoverConfig `mapstructure:"file-mover"`

	// RabbitMQ configuration
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq"`
}

// LogConfig contains logging configuration
type LogConfig struct {
	// Level is the logging level (debug, info, warn, error)
	Level string `mapstructure:"level"`

	// Directory is the directory where log files are stored
	Directory string `mapstructure:"directory"`
}

// CountryDirs contains directory configuration for a specific country
type CountryDirs struct {
	// TaxBill is the directory for tax bill files
	TaxBill string `mapstructure:"tax-bill"`

	// Export is the directory for export backup files
	Export string `mapstructure:"export"`
}

// ServiceDirs contains service directories for different countries
type ServiceDirs struct {
	// NL contains directory configuration for Netherlands
	NL CountryDirs `mapstructure:"nl"`

	// BE contains directory configuration for Belgium
	BE CountryDirs `mapstructure:"be"`
}

// ImportConfig contains import configuration
type ImportConfig struct {
	// XMLDir is the directory for imported XML files
	XMLDir string `mapstructure:"xml-dir"`
}

// CountryWatchConfig contains watch configuration for a specific country
type CountryWatchConfig struct {
	// Enabled indicates whether watching is enabled for this country
	Enabled bool `mapstructure:"enabled"`

	// KeepOriginal indicates whether to keep the original file
	KeepOriginal bool `mapstructure:"keep-original"`

	// WatchDir is the directory to watch for new files
	WatchDir string `mapstructure:"watch-dir"`

	// BackupDir is the directory for backups of processed files
	BackupDir string `mapstructure:"backup-dir"`
}

// ExportWatchConfig contains Export (XML) watch configuration
type ExportWatchConfig struct {
	// Enabled indicates whether Export watching is enabled
	Enabled bool `mapstructure:"enabled"`

	// NL contains watch configuration for Netherlands
	NL CountryWatchConfig `mapstructure:"nl"`

	// BE contains watch configuration for Belgium
	BE CountryWatchConfig `mapstructure:"be"`
}

// PdfWatchConfig contains PDF watch configuration
type PdfWatchConfig struct {
	// Enabled indicates whether PDF watching is enabled
	Enabled bool `mapstructure:"enabled"`

	// NL contains watch configuration for Netherlands
	NL CountryWatchConfig `mapstructure:"nl"`

	// BE contains watch configuration for Belgium
	BE CountryWatchConfig `mapstructure:"be"`
}

// WatchersConfig contains all watchers configuration
type WatchersConfig struct {
	// Export contains Export (XML) watch configuration
	Export ExportWatchConfig `mapstructure:"export"`

	// Pdf contains PDF watch configuration
	Pdf PdfWatchConfig `mapstructure:"pdf"`
}

// FileMoverConfig contains file mover configuration
type FileMoverConfig struct {
	// Enabled indicates whether file mover is enabled
	Enabled bool `mapstructure:"enabled"`

	// QueueSize is the size of the file removal queue
	QueueSize int `mapstructure:"queue-size"`

	// QueueName is the name of the file removal queue
	QueueName string `mapstructure:"queue-name"`

	// WorkerCount 工作线程数量，最大不超过5
	WorkerCount int `mapstructure:"worker-count"`
}

// RabbitMQConfig contains RabbitMQ configuration
type RabbitMQConfig struct {
	// URL is the RabbitMQ server URL
	URL string `mapstructure:"url"`

	// Heartbeat is the heartbeat interval
	Heartbeat string `mapstructure:"heartbeat"`

	// ConnectionTimeout is the connection timeout
	ConnectionTimeout string `mapstructure:"connection-timeout"`

	// MaxConnections is the maximum number of connections
	MaxConnections int `mapstructure:"max-connections"`

	// MaxChannelsPerConn is the maximum number of channels per connection
	MaxChannelsPerConn int `mapstructure:"max-channels-per-conn"`

	// AutoReconnect indicates whether to reconnect automatically
	AutoReconnect bool `mapstructure:"auto-reconnect"`

	// ReconnectInterval is the interval between reconnection attempts
	ReconnectInterval string `mapstructure:"reconnect-interval"`

	// PrefetchCount is the prefetch count for consumers
	PrefetchCount int `mapstructure:"prefetch-count"`

	// AutoAck indicates whether to auto-acknowledge messages
	AutoAck bool `mapstructure:"auto-ack"`

	// Import contains RabbitMQ configuration for importing
	Import RabbitMQImportConfig `mapstructure:"import"`

	// Export contains RabbitMQ configuration for exporting
	Export RabbitMQExportConfig `mapstructure:"export"`
}

// RabbitMQImportConfig contains RabbitMQ configuration for importing
type RabbitMQImportConfig struct {
	// Consumer is the consumer name
	Consumer string `mapstructure:"consumer"`

	// Exchange is the exchange name
	Exchange string `mapstructure:"exchange"`

	// ExchangeType is the exchange type
	ExchangeType string `mapstructure:"exchange-type"`

	// Queue is the queue name
	Queue string `mapstructure:"queue"`
}

// RabbitMQExportConfig contains RabbitMQ configuration for exporting
type RabbitMQExportConfig struct {
	// Exchange is the exchange name
	Exchange string `mapstructure:"exchange"`

	// ExchangeType is the exchange type
	ExchangeType string `mapstructure:"exchange-type"`

	// Queue is the queue name prefix
	Queue string `mapstructure:"queue"`
}

// InitConfig initializes the global configuration from viper
func InitConfig() error {
	// Create new configuration instance
	config := &AppConfig{}

	// Unmarshal configuration from viper
	if err := viper.Unmarshal(config); err != nil {
		return fmt.Errorf("unable to decode configuration: %v", err)
	}

	// Set global configuration
	GlobalConfig = config

	// 开启文件移动处理
	// 考虑到：
	// 1. 文件移动属于文件系统操作，可能在系统繁忙时，文件移动处理会阻塞，因此考虑异步处理
	// 2. 也就是所有涉及到文件系统中文件修改路径的操作，都应该放入此队列中交由异步队列处理
	InitFileMover()

	log.Infof("Configuration loaded successfully")
	return nil
}

// handleFileMover 文件移动处理函数
func handleFileMover(param string) error {
	log.Infof("[%s] 开始处理文件移动任务: %s", FileMoverQueue.Name, param)

	// 解析文件移动参数
	var fileMoverParam model.FileMoverParam
	if err := json.Unmarshal([]byte(param), &fileMoverParam); err != nil {
		log.Errorf("[%s] 解析文件移动参数失败: %v", FileMoverQueue.Name, err)
		return err
	}

	// 确保源文件和目标目录使用绝对路径
	sourceFile := fileMoverParam.SourceFile
	if !filepath.IsAbs(sourceFile) && util.IsExists(sourceFile) {
		sourceFile, _ = filepath.Abs(sourceFile)
	}

	// 确保目标目录是绝对路径
	moveTo := fileMoverParam.MoveTo
	if !filepath.IsAbs(moveTo) {
		absMoveTo, _ := filepath.Abs(moveTo)
		moveTo = absMoveTo
	}

	if fileMoverParam.IsCopy {
		// 复制文件
		if err := util.CopyFile(sourceFile, moveTo); err != nil {
			log.Errorf("[%s] 复制文件失败: %v", FileMoverQueue.Name, err)
			return err
		}
	} else {
		// 移动文件, 如果目标目录不存在则创建
		if err := util.MoveFile(sourceFile, moveTo, true); err != nil {
			log.Errorf("[%s] 移动文件失败: %v", FileMoverQueue.Name, err)
			return err
		}
	}

	log.Infof("[%s] 成功移动文件: %s -> %s", FileMoverQueue.Name, sourceFile, moveTo)

	return nil
}

// InitFileMover initializes file removal queues based on configuration
func InitFileMover() {
	// Initialize NL remover if enabled
	if GlobalConfig.FileMover.Enabled {
		queueSize := GlobalConfig.FileMover.QueueSize

		if queueSize <= 0 {
			queueSize = 500
		}

		queueName := GlobalConfig.FileMover.QueueName
		if queueName == "" {
			queueName = "global_async_file_mover"
		}

		workerCount := GlobalConfig.FileMover.WorkerCount
		if workerCount <= 0 {
			workerCount = 2 // 默认2个工作线程
		}

		FileMoverQueue = component.NewCustomQueue(queueSize, queueName, workerCount)

		log.Infof("启动文件移动队列 %s，缓冲大小: %d，工作线程数: %d", queueName, queueSize, workerCount)

		// 启动消费者处理函数
		FileMoverQueue.StartConsumer(handleFileMover)

		// 注意：这里不要调用Wait方法，否则会阻塞主线程
		// FileMoverQueue.Wait() - 不应在这里调用
	}
}

// PublishFileMover 发布文件移动消息
func PublishFileMover(param model.FileMoverParam) {
	msg, err := json.Marshal(param)
	if err != nil {
		log.Errorf("序列化文件移动参数失败: %v", err)
	}

	FileMoverQueue.Publish(string(msg))

}

// GetPort returns the HTTP server port with a default if not set
func (c *AppConfig) GetPort() string {
	if c.Port <= 0 {
		return "7003" // Default port
	}
	return fmt.Sprintf("%d", c.Port)
}

// GetLogLevel returns the log level with a default if not set
func (c *AppConfig) GetLogLevel() string {
	if c.Log.Level == "" {
		return "info" // Default log level
	}
	return c.Log.Level
}

// GetLogDirectory returns the log directory with a default if not set
func (c *AppConfig) GetLogDirectory() string {
	if c.Log.Directory == "" {
		return "out/log/" // Default log directory
	}
	return c.Log.Directory
}

// GetTempDir returns the temporary directory with a default if not set
func (c *AppConfig) GetTempDir() string {
	if c.TempDir == "" {
		return "out/tmp" // Default temporary directory
	}
	return c.TempDir
}

// GetExportWatchDir returns the Export watch directory for a specific country
func (c *AppConfig) GetExportWatchDir(country string) string {
	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Export.NL.WatchDir
	} else if country == "BE" {
		return c.Watchers.Export.BE.WatchDir
	} else {
		return ""
	}
}

// GetExportBackupDir returns the Export backup directory for a specific country
func (c *AppConfig) GetExportBackupDir(country string) string {
	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Export.NL.BackupDir
	} else if country == "BE" {
		return c.Watchers.Export.BE.BackupDir
	} else {
		return ""
	}
}

// GetPdfWatchDir returns the PDF watch directory for a specific country
func (c *AppConfig) GetPdfWatchDir(country string) string {
	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Pdf.NL.WatchDir
	} else if country == "BE" {
		return c.Watchers.Pdf.BE.WatchDir
	} else {
		return ""
	}
}

// GetPdfBackupDir returns the PDF backup directory for a specific country
func (c *AppConfig) GetPdfBackupDir(country string) string {
	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Pdf.NL.BackupDir
	} else if country == "BE" {
		return c.Watchers.Pdf.BE.BackupDir
	} else {
		return ""
	}
}

// IsExportWatcherEnabled checks if Export watcher is enabled for a specific country
func (c *AppConfig) IsExportWatcherEnabled(country string) bool {
	if !c.Watchers.Export.Enabled {
		return false
	}

	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Export.NL.Enabled
	} else if country == "BE" {
		return c.Watchers.Export.BE.Enabled
	} else {
		return false
	}
}

// IsPdfWatcherEnabled checks if PDF watcher is enabled for a specific country
func (c *AppConfig) IsPdfWatcherEnabled(country string) bool {
	if !c.Watchers.Pdf.Enabled {
		return false
	}

	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Pdf.NL.Enabled
	} else if country == "BE" {
		return c.Watchers.Pdf.BE.Enabled
	} else {
		return false
	}
}

// IsKeepOriginalEnabled checks if keep original is enabled for a specific country
func (c *AppConfig) IsKeepOriginalEnabled(country string) bool {
	if !c.Watchers.Pdf.Enabled {
		return false
	}

	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Pdf.NL.KeepOriginal
	} else if country == "BE" {
		return c.Watchers.Pdf.BE.KeepOriginal
	} else {
		return false
	}

}

// GetTaxBillDir returns the tax bill directory for a specific country
func (c *AppConfig) GetTaxBillDir(country string) string {
	country = normalizeCountry(country)

	if country == "NL" {
		return c.Watchers.Pdf.NL.BackupDir
	} else if country == "BE" {
		return c.Watchers.Pdf.BE.BackupDir
	} else {
		return ""
	}
}

// GetImportXMLDir returns the import XML directory
func (c *AppConfig) GetImportXMLDir() string {
	return c.Import.XMLDir
}

// normalizeCountry normalizes country code to uppercase
func normalizeCountry(country string) string {
	return strings.ToUpper(country)
}
