package handler

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/service"
)

// TaxBillHandler 税金单处理器
type TaxBillHandler struct {
	taxBillService *service.TaxBillService
}

// NewTaxBillHandler 创建税金单处理器
func NewTaxBillHandler() *TaxBillHandler {
	return &TaxBillHandler{
		taxBillService: service.NewTaxBillService(),
	}
}

// DownloadTaxBill 下载税金单文件
// @Summary      下载税金单文件
// @Description  下载指定国家的税金单文件
// @Tags         tax-bill
// @Accept       json
// @Produce      octet-stream
// @Param        country  path  string  true  "国家代码(NL|BE)"
// @Param        filename  path  string  true  "文件名"
// @Success      200 {file} binary "文件内容"
// @Failure      400 {object} model.ResponseError
// @Failure      404 {object} model.ResponseError
// @Failure      500 {object} model.ResponseError
// @Router       /api/tax-bills/{country}/download/{filename} [get]
func (h *TaxBillHandler) DownloadTaxBill(c echo.Context) error {
	country := strings.ToUpper(c.Param("country"))
	filename := c.Param("filename")

	if country == "" || filename == "" {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: []string{"国家和文件名参数不能为空"},
		})
	}

	// 检查国家是否支持
	if country != "NL" && country != "BE" {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: []string{"不支持的国家代码，只支持 NL 或 BE"},
		})
	}

	// 安全检查，防止目录遍历攻击
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: []string{"无效的文件名格式"},
		})
	}

	// 使用服务层方法查找文件
	filePath, err := h.taxBillService.FindTaxBillFile(filename, country)
	if err != nil {
		log.Errorf("查找税金单文件失败: %v", err)
		return c.JSON(http.StatusNotFound, &model.ResponseError{
			Status: model.FAIL,
			Errors: []string{err.Error()},
		})
	}

	log.Infof("下载税金单文件: %s", filePath)

	// 设置响应头，指定文件类型
	if strings.HasSuffix(strings.ToLower(filename), ".pdf") {
		c.Response().Header().Set("Content-Type", "application/pdf")
	} else {
		c.Response().Header().Set("Content-Type", "application/octet-stream")
	}

	return c.Attachment(filePath, filename)
}
