package service

import (
	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
	"os"
	"regexp"
	"time"
)

/**
监听import 文件路径下，文件的创建
*/

// RE_XML_FILE XML file
const (
	RE_XML_FILE = ".*\\.xml"
)

// Watch Listen the directory for declare country
func Watch(dir string, dc string) {
	log.Debug("Watch dir: ", dir)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("New file watcher error:", err)
	}
	defer func(watcher *fsnotify.Watcher) {
		err := watcher.Close()
		if err != nil {
			log.Panic("Close fsnotify watcher, error:", err)
		}
	}(watcher)

	done := make(chan bool)
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:

				// Channel was closed (i.e. Watcher.Close() was called).
				if !ok {
					log.Error("Event Channel was closed (i.e. Watcher.Close() was called):", err)
					return
				}
				log.Printf("File:%s is  %s", event.Name, event.Op)
				// (event.Op&fsnotify.Write == fsnotify.Write)
				// 只监听创建，创建时判断文件是否可读
				if event.Op&fsnotify.Create == fsnotify.Create {
					compileXml := regexp.MustCompile(RE_XML_FILE)
					filename := event.Name

					if compileXml.MatchString(filename) {
						canRead, _ := waitFileWriteFinish(filename, 10)
						if canRead {
							SendExportXml(filename, dc)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				// Channel was closed (i.e. Watcher.Close() was called).
				if !ok {
					log.Error("File watcher Error Channel was closed (i.e. Watcher.Close() was called):", err)
					return
				}
				log.Error("File read error:", err)
			}
		}
	}()

	err = watcher.Add(dir)

	if err != nil {
		log.Fatal(err)
	}
	<-done
}

// waitFileWriteFinish 检查文件是否有内容，内容长度小于100任务没有内容，等待5秒重新读取，直到文件有内容产生
func waitFileWriteFinish(filename string, count int) (bool, error) {
	if count == 0 {
		log.Errorf("File: %s content is always writing, can't be read.'", filename)
		return false, nil
	}
	count = count - 1
	content, err := os.ReadFile(filename)
	if err != nil {
		log.Errorf("File: %s read failed. error: %v", filename, err)
		return false, err
	}

	if len(content) < 100 {
		log.Warnf("File: %s is writing wait 5 seconds...", filename)

		time.Sleep(5 * time.Second)
		return waitFileWriteFinish(filename, count)
	}

	return true, nil
}
