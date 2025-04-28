/*
Copyright © 2022 Joker
*/
package main

import (
	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/cmd"
)

func init() {
	log.SetLevel(log.InfoLevel)
}

func main() {
	cmd.Execute()
}
