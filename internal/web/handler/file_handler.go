package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"sysafari.com/softpak/rattler/internal/util"
)

// DownloadTaxPdf Download the export PDF file
// @Summary      下载税金单文件
// @Description  通过申报国家确定税金文件路径
// @Tags         download
// @Accept       json
// @Produce      json
// @Param        origin   path  string   true  "下载的源文件名,不带文件后缀"
// @Param        target   path  string   true  "下载文件后，将文件重命名的文件名，没有后缀将自动添加pdf作为后缀"
// @Param        dc   	  query string   false  "申报国家(BE|NL),默认为NL"
// @Success      200
// @Failure      400
// @Failure      404
// @Failure      500
// @Router       /download/pdf/{origin}/{target} [get]
func DownloadTaxPdf(c echo.Context) error {
	nlTaxDir := viper.GetString("ser-dir.nl.tax-bill")
	beTaxDir := viper.GetString("ser-dir.be.tax-bill")

	origin := c.Param("origin") + ".pdf"
	target := c.Param("target")
	if !strings.Contains(target, ".pdf") {
		target = target + ".pdf"
	}

	dc := strings.ToUpper(c.QueryParam("dc"))

	var filePath string
	// dc 为空则为nl
	if dc == "BE" {
		filePath = filepath.Join(beTaxDir, origin)
	} else {
		filePath = filepath.Join(nlTaxDir, origin)
	}

	if util.IsExists(filePath) {
		return c.Attachment(filePath, target)
	}

	log.Errorf("Download tax-bill pdf failed,%s is not found", filePath)

	return c.String(http.StatusNotFound,
		fmt.Sprintf("Download tax-bill pdf failed,%s is not found.", origin))
}

// DownloadExportXml Download the export XML file
// @Summary      下载export文件
// @Description  通过文件名前缀确定文件路径
// @Tags         download
// @Accept       json
// @Produce      json
// @Param        dc   	  path  string   true  "申报国家(BE|NL)"
// @Param        filename path  string   true  "export文件的文件名"
// @Param        download query string   false  "是否下载，1表示直接下载"
// @Success      200
// @Failure      400
// @Failure      404
// @Failure      500
// @Router       /download/xml/{dc}/{filename} [get]
func DownloadExportXml(c echo.Context) error {
	nlExportDir := viper.GetString("ser-dir.nl.export")
	beExportDir := viper.GetString("ser-dir.be.export")

	dc := strings.ToUpper(c.Param("dc"))
	filename := c.Param("filename")

	year := filename[0:4]
	month := filename[4:6]

	needDownload := c.QueryParam("download")

	var filePath string
	if dc == "NL" {
		filePath = filepath.Join(nlExportDir, year, month, filename)
	} else if dc == "BE" {
		filePath = filepath.Join(beExportDir, filename)
	} else {
		return c.String(http.StatusNotFound, fmt.Sprintf("%s is not a valid declare country", dc))
	}

	fmt.Println("download filePath:", filePath)
	if util.IsExists(filePath) {
		if needDownload == "1" {
			return c.Attachment(filePath, filename)
		}
		return c.File(filePath)
	}

	log.Errorf("Download export xl failed,%s is not found.", filePath)

	return c.String(http.StatusNoContent, fmt.Sprintf("The file %s is not found", filename))
}
