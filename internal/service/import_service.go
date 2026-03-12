// Package service 报关文件import xml 服务类
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/util"
)

// ImportDocument Import xml document request param
type ImportDocument struct {
	Filename string `json:"filename"`
	Document string `json:"document"`
}

// SaveImportDocument 保存import XML文档
func SaveImportDocument(message string) {
	// 去除转义符
	msg, err := strconv.Unquote(message)
	doc := ImportDocument{}
	if err != nil {
		err = json.Unmarshal([]byte(message), &doc)
	} else {
		err = json.Unmarshal([]byte(msg), &doc)
	}

	if err != nil {
		log.Errorf("解析队列消息失败: %v", err)
		fmt.Println("解析队列消息失败: ", err)
		return
	}

	filename := doc.Filename
	document := doc.Document
	importDir := config.GlobalConfig.GetImportXMLDir()

	// 确保导入目录存在
	if importDir == "" {
		log.Errorf("导入目录配置为空")
		return
	}

	// 创建目录（如果不存在）
	canSave := util.IsDir(importDir) || util.CreateDir(importDir)
	if !canSave {
		log.Errorf("导入目录 %s 不存在且无法创建，无法保存导入XML文件", importDir)
		return
	}

	// 写入文件
	fp := filepath.Join(importDir, filename)
	err = os.WriteFile(fp, []byte(document), os.ModePerm)
	if err != nil {
		log.Errorf("写入文件 %s 失败: %v", fp, err)
	} else {
		log.Infof("成功写入文件: %s", fp)
	}
}
