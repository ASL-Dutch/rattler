package service

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/rabbit"
	"sysafari.com/softpak/rattler/internal/util"
)

// 报关结果放行文件服务类

type WatchConfig struct {
	Watch     bool
	WatchDir  string
	BackupDir string
}

// Dc Declare country
type Dc uint32

// ExportXmlInfo Export XML file information
type ExportXmlInfo struct {
	FileName       string `json:"fileName"`
	DeclareCountry string `json:"declareCountry"`
	Content        string `json:"content"`
}

// SendExportXml sends export Xml file to the MQ
// Compress the content of the XML file before sending,
// and then create a json object and send it to the message queue
func SendExportXml(filename string, declareCountry string) {
	content, err := readStableXMLContent(filename)
	if err != nil {
		log.Errorf("读取稳定XML文件失败，不发送MQ: %v", err)
		return
	}
	compressedXml := util.AdvancedCompressXML(string(content))

	// 获取原始文件大小用于对比
	originalSize := int64(len(content))
	compressedSize := int64(len(compressedXml))

	// 记录压缩前后的大小差异
	if originalSize > compressedSize {
		log.Infof("XML压缩: 原始大小 %d 字节, 压缩后 %d 字节, 减少了 %.2f%%",
			originalSize, compressedSize, float64(originalSize-compressedSize)*100/float64(originalSize))
	} else {
		log.Debugf("XML压缩: 无效果或增加了大小")
	}

	log.Debugf("Min size xml content:  %s ", compressedXml)

	// backup export xml
	fn, err := moveFileToBackup(filename, declareCountry)
	if err != nil {
		// Backup failed send original file name
		fn = filepath.Base(filename)
	}

	xmlContent := ExportXmlInfo{
		FileName:       fn,
		DeclareCountry: declareCountry,
		Content:        compressedXml,
	}
	// Serialize to JSON
	bf := bytes.NewBuffer([]byte{})
	jsonEncoder := json.NewEncoder(bf)
	jsonEncoder.SetEscapeHTML(false)
	err = jsonEncoder.Encode(xmlContent)

	if err != nil {
		log.Error("Serialize Export xml file to JSON failed, dont publish. ", err)
	} else {
		//jobNumber, _ := getJobNumber(filename)
		// Send xml info to MQ
		publishMessageToMQ(bf.String(), declareCountry)
	}
}

// readStableXMLContent 读取已稳定且结构完整的XML内容，避免监听创建事件后读取到半文件
func readStableXMLContent(filename string) ([]byte, error) {
	maxAttempts := 0
	checkIntervalMs := 0
	var minContentSize int64
	if config.GlobalConfig != nil {
		maxAttempts = config.GlobalConfig.Watchers.Export.XMLReadiness.MaxAttempts
		checkIntervalMs = config.GlobalConfig.Watchers.Export.XMLReadiness.CheckIntervalMs
		minContentSize = config.GlobalConfig.Watchers.Export.XMLReadiness.MinContentSize
	}

	// 默认值兜底，避免未配置导致行为异常
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	if checkIntervalMs <= 0 {
		checkIntervalMs = 1500
	}
	if minContentSize <= 0 {
		minContentSize = 16
	}

	const stabilityRequiredHits = 2
	checkInterval := time.Duration(checkIntervalMs) * time.Millisecond

	var (
		lastSize  int64 = -1
		stableHit int
		lastErr   error
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		content, err := os.ReadFile(filename)
		if err != nil {
			lastErr = err
			log.Debugf("读取XML失败，等待重试(%d/%d): %s, err=%v", attempt, maxAttempts, filename, err)
			time.Sleep(checkInterval)
			continue
		}

		size := int64(len(content))
		if size < minContentSize {
			lastErr = fmt.Errorf("文件内容过小: %d", size)
			log.Debugf("XML内容过小，等待重试(%d/%d): %s, size=%d", attempt, maxAttempts, filename, size)
			time.Sleep(checkInterval)
			continue
		}

		// 连续两次大小一致视为“写入稳定”，可显著降低读取半文件概率
		if size == lastSize {
			stableHit++
		} else {
			stableHit = 1
			lastSize = size
		}

		if stableHit < stabilityRequiredHits {
			time.Sleep(checkInterval)
			continue
		}

		if !isWellFormedXML(content) {
			lastErr = fmt.Errorf("XML结构尚未完整")
			log.Debugf("XML结构不完整，等待重试(%d/%d): %s", attempt, maxAttempts, filename)
			time.Sleep(checkInterval)
			continue
		}

		return content, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("文件在重试后仍未稳定")
	}
	return nil, fmt.Errorf("读取稳定XML失败: %s, err=%w", filename, lastErr)
}

// isWellFormedXML 判断XML是否可完整解析（只校验结构，不做业务字段校验）
func isWellFormedXML(content []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	decoder.CharsetReader = util.CharsetReader
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			return true
		}
		if err != nil {
			return false
		}
	}
}

// publishMessageToMQ publishes the message to MQ
func publishMessageToMQ(message string, declareCountry string) {
	// 从全局配置获取参数
	qPrefix := config.GlobalConfig.RabbitMQ.Export.Queue
	var queueName = strings.ToLower(qPrefix + "." + declareCountry)

	exchange := config.GlobalConfig.RabbitMQ.Export.Exchange

	// 获取RabbitMQ管理器实例
	manager, err := rabbit.GetInstance()
	if err != nil {
		log.Errorf("Failed to get RabbitMQ manager: %v", err)
		return
	}

	// 使用管理器发布消息
	err = manager.PublishMessage(exchange, queueName, message)
	if err != nil {
		log.Errorf("Failed to publish message to queue %s: %v", queueName, err)
	} else {
		log.Infof("Successfully published message to queue %s", queueName)
	}
}

// moveFileToBackup Move file to back up location
func moveFileToBackup(fp string, dc string) (string, error) {
	fn := filepath.Base(fp)

	var year, month, newFileName string

	firstPt := strings.Split(fn, "_")[0]
	parse, err := time.Parse("200601", firstPt)
	if err != nil {
		year = time.Now().Format("2006")
		month = time.Now().Format("01")
		newFileName = fmt.Sprintf("%s%s_%s", year, month, fn)
	} else {
		log.Warnf("文件:%s 在路径 %s 下, 备份是原始文件名.", fn, parse.Format("2006-01-02"))
		year = parse.Format("2006")
		month = parse.Format("01")
		newFileName = fn
	}

	// 从全局配置获取备份目录
	backupDir := config.GlobalConfig.GetExportBackupDir(dc)
	if backupDir == "" {
		log.Errorf("申报国家 %s 的备份目录未配置", dc)
		return "", fmt.Errorf("申报国家 %s 的备份目录未配置", dc)
	}

	bacdir := filepath.Join(backupDir, year, month)

	fileMoverParam := model.FileMoverParam{
		SourceFile: fp,
		MoveTo:     filepath.Join(bacdir, newFileName),
	}

	config.PublishFileMover(fileMoverParam)

	return newFileName, nil
}

// ExportListenDicFiles 获取申报国家Export 监听路径下的文件列表
func ExportListenDicFiles(dc string) (files []model.ExportFileListDTO, err error) {
	// 从全局配置获取监听目录
	listenDir := config.GlobalConfig.GetExportWatchDir(dc)
	if listenDir == "" {
		return nil, fmt.Errorf("申报国家 %s 的监听目录未配置", dc)
	}

	log.Debugf("获取 %s 监听目录下的文件: %s", dc, listenDir)
	if !util.IsDir(listenDir) || !util.IsExists(listenDir) {
		return nil, errors.New("the monitoring path is wrong. Check whether the declared country exists")
	}

	// 获取文件列表
	var fs []string
	err = filepath.Walk(listenDir, util.Visit(&fs))
	if err != nil {
		return nil, err
	}
	log.Debugf("发现文件: %v", fs)

	for _, f := range fs {
		info, err := os.Stat(f)
		if err == nil {
			ef := model.ExportFileListDTO{
				Filename: filepath.Base(f),
				Filepath: "",
				Size:     info.Size(),
				ModTime:  info.ModTime().Format("2006-01-02 15:04:05"),
			}
			absPath, err := filepath.Abs(f)
			if err != nil {
				ef.Filepath = f
			} else {
				ef.Filepath = absPath
			}
			files = append(files, ef)
		} else {
			log.Errorf("获取文件 %s 的 stat 失败, error: %v", f, err)
		}
	}

	return files, err
}
