package config

import (
	"sysafari.com/softpak/rattler/internal/component"
)

// global params

// 两个申报国家的remover进程，由于需要在service中使用
// 所以将remover的指针声明为全局变量
var (
	// NlRemover Export监听目录的remover
	NlRemover *component.RemoveQueue

	// BeRemover BE监听目录的remover
	BeRemover *component.RemoveQueue
)
