package util

import (
	log "github.com/sirupsen/logrus"
	"os"
	"path/filepath"
)

// IsDir Path is directory
func IsDir(fileAddr string) bool {
	s, err := os.Stat(fileAddr)
	if err != nil {
		log.Println(err)
		return false
	}
	return s.IsDir()
}

// CreateDir creates a directory
func CreateDir(dir string) bool {
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		log.Println(err)
		return false
	}
	return true
}

// IsExists Path is exists
func IsExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	return false
}

// Visit Visit directory get file path
func Visit(files *[]string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
		}
		if !IsDir(path) {
			*files = append(*files, path)
		}
		return nil
	}
}
