package config

import (
	"io"
	"os"
	"path/filepath"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	log "github.com/sirupsen/logrus"
)

// InitLog Initialize log settings
func InitLog(logFilename string, lev string) {
	log.Debugf("LOG filename: %s, level: %s", logFilename, lev)
	if logFilename == "" {
		path, _ := os.Executable()
		_, exec := filepath.Split(path)
		logFilename = exec + ".log"
		log.Warning("LOG filename is empty, log info save in current path(rattler.log).")
	}

	// Set to generate a log file every day
	// Keep logs for 15 days
	writer, _ := rotatelogs.New(logFilename+".%Y%m%d",
		rotatelogs.WithLinkName(logFilename),
		rotatelogs.WithRotationCount(15),
		rotatelogs.WithRotationTime(time.Duration(24)*time.Hour))

	// 设置日志同时输出到终端和文件
	mw := io.MultiWriter(os.Stdout, writer)
	log.SetOutput(mw)

	// 设置格式化器，使日期更易读
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,                      // 显示完整时间戳
		TimestampFormat: "2006-01-02 15:04:05.000", // 更易读的时间格式
		DisableQuote:    true,                      // 不给字段值加引号
	})

	level, err := log.ParseLevel(lev)
	if err == nil {
		log.SetLevel(level)
	}
}
