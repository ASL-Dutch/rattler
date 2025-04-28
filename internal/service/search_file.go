package service

import (
	"fmt"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/util"
)

// SearchFile Search softpak file
type SearchFile struct {
	DeclareCountry string `json:"declare_country"`
	// Year exp: 2022
	Year string `json:"year"`
	// Month exp: 09
	Month string `json:"month"`
	// Type TAX_BILL, EXPORT_XML
	Type         string                   `json:"type"`
	Directory    string                   `json:"directory"`
	Filenames    []string                 `json:"filenames"`
	SearchResult []model.SearchFileResult `json:"searchResult"`
	Errors       []string                 `json:"errors"`
}

// ready Ready for search
func (sf *SearchFile) ready() {
	fmt.Println(sf.DeclareCountry)
	if "NL" == sf.DeclareCountry {
		if "TAX_BILL" == sf.Type {
			sf.Directory = viper.GetString("ser-dir.nl.tax-bill")
		}
		if "EXPORT_XML" == sf.Type {
			sf.Directory = viper.GetString("watcher.nl.backup-dir")
		}
	}

	if "BE" == sf.DeclareCountry {
		if "TAX_BILL" == sf.Type {
			sf.Directory = viper.GetString("ser-dir.be.tax-bill")
		}
		if "EXPORT_XML" == sf.Type {
			sf.Directory = viper.GetString("watcher.be.backup-dir")
		}
	}
	if sf.Year != "" {
		sf.Directory = filepath.Join(sf.Directory, sf.Year)
	}

	// month 路径必须在year 路径后
	if sf.Year != "" && sf.Month != "" {
		sf.Directory = filepath.Join(sf.Directory, sf.Month)
	}

	fmt.Println(sf.Directory)
	if !util.IsDir(sf.Directory) || !util.IsExists(sf.Directory) {
		sf.Errors = append(sf.Errors, fmt.Sprintf("The file directory %s not exists", sf.Directory))
	}
}

// search Start to search file
func (sf *SearchFile) search() {
	if len(sf.Errors) > 0 {
		return
	}

	var files []string
	err := filepath.Walk(sf.Directory, util.Visit(&files))
	if err != nil {
		sf.Errors = append(sf.Errors, fmt.Sprintf("Failed to get all file names through the directory:%s", sf.Directory))
		return
	}
	log.Infof("The directory: %s contains %d files.", sf.Directory, len(files))

	for _, file := range files {
		filename := filepath.Base(file)
		for _, s := range sf.Filenames {
			if strings.Contains(filename, s) {
				sfr := model.SearchFileResult{
					Type:       sf.Type,
					SearchText: s,
					Filename:   filename,
					Filepath:   "",
				}
				absPath, err := filepath.Abs(file)
				if err != nil {
					sfr.Filepath = file
				} else {
					sfr.Filepath = absPath
				}
				sf.SearchResult = append(sf.SearchResult, sfr)
				break
			}
		}
	}
}

// GetSearchResult Begin to search
func (sf *SearchFile) GetSearchResult() ([]model.SearchFileResult, []string) {
	sf.ready()
	sf.search()

	return sf.SearchResult, sf.Errors
}
