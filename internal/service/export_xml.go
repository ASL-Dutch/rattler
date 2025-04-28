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
	"github.com/spf13/viper"
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
	// Get all the required config parameters
	qPrefix := viper.GetString("rabbitmq.export.queue")
	var queueName = strings.ToLower(qPrefix + "." + declareCountry)

	exchange := viper.GetString("rabbitmq.export.exchange")

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

	firstPt := strings.Split(fn, "_")[0]
	parse, err := time.Parse("200601", firstPt)
	var year, month, newFileName string
	if err != nil {
		year = time.Now().Format("2006")
		month = time.Now().Format("01")
		newFileName = fmt.Sprintf("%s%s_%s", year, month, fn)
	} else {
		log.Warnf("The file:%s within date ,backup is origin filename.", fn)
		year = parse.Format("2006")
		month = parse.Format("01")
		newFileName = fn
	}

	backupDir := viper.GetString(fmt.Sprintf("watcher.%s.backup-dir", strings.ToLower(dc)))
	bacdir := filepath.Join(backupDir, year, month)
	// backup directory not exists create it
	canMove := util.IsDir(bacdir) || util.CreateDir(bacdir)
	if !canMove {
		log.Errorf("Cannot create backup dir %s , dont move file %s", bacdir, fp)
		return "", fmt.Errorf("cannot create backup dir %s , dont move file %s", bacdir, fp)
	}
	filename := filepath.Base(fp)
	targetFilename := filepath.Join(bacdir, newFileName)

	err = os.Rename(fp, targetFilename)
	if err != nil {
		log.Errorf("Backup export file %s failed, error: %v", filename, err)
		return "", err
	}
	return newFileName, nil
}

// ExportListenDicFiles 获取申报国家Export 监听路径下的文件列表
func ExportListenDicFiles(dc string) (files []model.ExportFileListDTO, err error) {
	var listenDir string
	if dc == "NL" {
		listenDir = viper.GetString("watcher.nl.watch-dir")
	}
	if dc == "BE" {
		listenDir = viper.GetString("watcher.be.watch-dir")
	}
	fmt.Println("listenDir", listenDir)
	if !util.IsDir(listenDir) || !util.IsExists(listenDir) {
		return nil, errors.New("the monitoring path is wrong. Check whether the declared country exists")
	}

	// 获取文件列表
	var fs []string
	err = filepath.Walk(listenDir, util.Visit(&fs))
	if err != nil {
		return nil, err
	}
	fmt.Println("fs", fs)

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
			log.Errorf("File: %s get stat failed, error: %v", f, err)
		}
	}

	return files, err
}
