package util

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	return err == nil
}

// Visit Visit directory get file path
func Visit(files *[]string) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !IsDir(path) {
			*files = append(*files, path)
		}
		return nil
	}
}

// MoveFile 移动文件
// 移动前判断源文件是否存在，不存在则报错
// target参数说明：
// 1. 如果target以路径分隔符结尾或是已存在的目录，则保留源文件名
// 2. 如果target包含文件名部分，则使用指定的文件名
// 3. 如果isMkdir为true则目录不存在时会批量创建
func MoveFile(srcFile, target string, isMkdir bool) error {
	fmt.Printf("srcFile: %s, target: %s, isMkdir: %v\n", srcFile, target, isMkdir)

	// 检查源文件是否存在
	if !IsExists(srcFile) {
		err := os.ErrNotExist
		return err
	}

	// 判断源路径是否为目录
	if IsDir(srcFile) {
		err := os.ErrInvalid
		return fmt.Errorf("源路径不应为目录: %v", err)
	}

	// 判断目标是目录还是文件路径
	var targetDir string
	var targetPath string

	// 获取源文件名
	srcFileName := filepath.Base(srcFile)

	// 检查target是否为已存在的目录
	if IsDir(target) {
		// 如果目标是已存在的目录，直接使用该目录加源文件名
		targetDir = target
		targetPath = filepath.Join(targetDir, srcFileName)
	} else if strings.HasSuffix(target, string(os.PathSeparator)) {
		// 如果目标以路径分隔符结尾，视为目录
		targetDir = target
		targetPath = filepath.Join(targetDir, srcFileName)
	} else {
		// 否则，认为用户指定了完整路径（包括目标文件名）
		targetDir = filepath.Dir(target)
		targetPath = target // 保持用户指定的目标文件名
	}

	// 确保目标目录存在
	if !IsExists(targetDir) {
		if isMkdir {
			if !CreateDir(targetDir) {
				err := os.ErrPermission
				return fmt.Errorf("无法创建目标目录: %v", err)
			}
		} else {
			err := os.ErrNotExist
			return fmt.Errorf("目标目录不存在: %v", err)
		}
	}

	// 执行移动操作
	err := os.Rename(srcFile, targetPath)
	if err != nil {
		return fmt.Errorf("移动文件失败: %v", err)
	}

	return nil
}

// copyFileContent 复制文件内容
// 内部函数，将源文件内容复制到目标文件
func copyFileContent(srcPath, destPath string) error {
	// 打开源文件
	sourceFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %v", err)
	}
	defer sourceFile.Close()

	// 获取源文件信息，用于后续设置权限
	sourceFileInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("获取源文件信息失败: %v", err)
	}

	// 创建目标文件
	destFile, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %v", err)
	}
	defer destFile.Close()

	// 执行文件复制
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("复制文件内容失败: %v", err)
	}

	// 确保所有内容都写入磁盘
	err = destFile.Sync()
	if err != nil {
		return fmt.Errorf("将文件内容同步到磁盘失败: %v", err)
	}

	// 设置目标文件权限与源文件相同
	err = os.Chmod(destPath, sourceFileInfo.Mode())
	if err != nil {
		log.Printf("设置目标文件权限失败: %v", err)
		// 继续执行，不中断复制过程
	}

	return nil
}

// CopyFile 复制文件
// target参数可以是一个目录或者包含文件名的完整路径
// 如果target是目录或者以路径分隔符结尾，则使用源文件名
// 如果target目录不存在，会自动创建
func CopyFile(srcFile, target string) error {
	// 检查源文件是否存在
	if !IsExists(srcFile) {
		return fmt.Errorf("源文件不存在: %s", srcFile)
	}

	// 判断源文件是否为目录
	if IsDir(srcFile) {
		return fmt.Errorf("源文件不能是目录")
	}

	// 获取源文件名
	srcFileName := filepath.Base(srcFile)

	// 确定目标路径
	var targetPath string

	// 检查target是否为目录、以分隔符结尾，或是具体文件路径
	if IsDir(target) {
		// 目标是已存在的目录
		targetPath = filepath.Join(target, srcFileName)
	} else if strings.HasSuffix(target, string(os.PathSeparator)) {
		// 目标以路径分隔符结尾，视为目录
		targetPath = filepath.Join(target, srcFileName)
	} else {
		// 目标指定了完整路径（包括文件名）
		targetPath = target
	}

	// 确保目标目录存在
	targetDir := filepath.Dir(targetPath)
	if !IsExists(targetDir) {
		if !CreateDir(targetDir) {
			return fmt.Errorf("无法创建目标目录: %s", targetDir)
		}
	}

	// 执行文件内容复制
	return copyFileContent(srcFile, targetPath)
}
