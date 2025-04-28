package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	log.Infof("Declare country: %s export xml: %s reading ", declareCountry, filename)

	// 优先使用文件级别压缩
	compressedXml, err := util.CompressXMLFile(filename)
	if err != nil {
		log.Warnf("文件级压缩失败，尝试使用常规方法: %v", err)

		// 如果文件级压缩失败，回退到常规方法
		content, err := os.ReadFile(filename)
		if err != nil {
			log.Error("Read XML file error:", err)
			return
		}
		compressedXml = util.AdvancedCompressXML(string(content))
	}

	// 获取原始文件大小用于对比
	fileInfo, err := os.Stat(filename)
	if err == nil {
		originalSize := fileInfo.Size()
		compressedSize := int64(len(compressedXml))

		// 记录压缩前后的大小差异
		if originalSize > compressedSize {
			log.Infof("XML压缩: 原始大小 %d 字节, 压缩后 %d 字节, 减少了 %.2f%%",
				originalSize, compressedSize, float64(originalSize-compressedSize)*100/float64(originalSize))
		} else {
			log.Debugf("XML压缩: 无效果或增加了大小")
		}
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
