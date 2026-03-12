package handler

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
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
	origin := c.Param("origin") + ".pdf"
	target := c.Param("target")
	if !strings.Contains(target, ".pdf") {
		target = target + ".pdf"
	}

	dc := strings.ToUpper(c.QueryParam("dc"))
	if dc == "" {
		dc = "NL" // 默认为NL
	}

	// 使用配置对象获取路径
	var filePath string
	taxBillDir := config.GlobalConfig.GetStorageTaxBillDir(dc)
	if taxBillDir == "" {
		return c.String(http.StatusNotFound,
			fmt.Sprintf("未配置申报国家 %s 的税单目录", dc))
	}

	filePath = filepath.Join(taxBillDir, origin)

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
	dc := strings.ToUpper(c.Param("dc"))
	filename := c.Param("filename")
	needDownload := c.QueryParam("download")

	// 使用配置对象获取路径
	exportDir := config.GlobalConfig.GetStorageExportDir(dc)
	if exportDir == "" {
		return c.String(http.StatusNotFound,
			fmt.Sprintf("%s is not a valid declare country or export directory not configured", dc))
	}

	// 解析文件名中的年月
	var filePath string
	if len(filename) >= 6 {
		year := filename[0:4]
		month := filename[4:6]
		filePath = filepath.Join(exportDir, year, month, filename)
	} else {
		// 对于不符合格式的文件名，直接在导出目录下查找
		filePath = filepath.Join(exportDir, filename)
	}

	log.Debugf("Download export XML file path: %s", filePath)
	if util.IsExists(filePath) {
		if needDownload == "1" {
			return c.Attachment(filePath, filename)
		}
		return c.File(filePath)
	}

	log.Errorf("Download export XML failed, %s is not found.", filePath)
	return c.String(http.StatusNoContent, fmt.Sprintf("The file %s is not found", filename))
}
