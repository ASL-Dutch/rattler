package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/service"
)

// ExportListenFiles 获取Export监听路径下当前文件列表
// @Summary      获取Export监听路径下当前文件列表
// @Description  通过指定的申报国家获取其当前Export监听路径下的文件列表
// @Tags         export
// @Accept       json
// @Produce      json
// @Param        dc  path  	  string   true  "申报国家(BE|NL)"
// @Success      200 {object} []softpak.ExportFileListDTO
// @Failure      400 {object} util.ResponseError
// @Failure      404 {object} util.ResponseError
// @Failure      500 {object} util.ResponseError
// @Router       /export/list/{dc} [get]
func ExportListenFiles(c echo.Context) (err error) {
	dc := strings.ToUpper(c.Param("dc"))
	fmt.Println(dc)

	data, err := service.ExportListenDicFiles(dc)
	if err != nil {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: []string{
				err.Error(),
			},
		})
	}

	// success
	return c.JSON(http.StatusOK, data)
}

// ExportFileResend 重新发送Export XML 文件
// @Summary      重新发送Export XML 文件
// @Description  页面选取Export文件并发送文件完整路径，将Export文件重新移入Export监听路径中，触发文件的CREATE监听，从而重新发送
// @Tags         export
// @Accept       json
// @Produce      json
// @Param        dc   	  path  string   true  "申报国家(BE|NL)"
// @Param        message body   FileResendRequest true  "需要重新发送的文件完整路径"
// @Success      200 {object} []softpak.ExportFileListDTO
// @Failure      400 {object} util.ResponseError
// @Failure      404 {object} util.ResponseError
// @Failure      500 {object} util.ResponseError
// @Router       /export/remover/{dc} [post]
func ExportFileResend(c echo.Context) (err error) {
	dc := strings.ToUpper(c.Param("dc"))
	if dc == "" {
		return c.JSON(http.StatusBadRequest, "The declare country is required.")
	}

	var errs []string
	sfd := new(model.FileResendRequest)
	if err = c.Bind(sfd); err != nil {
		errs = append(errs, err.Error())
	}
	// valiator 必须提前绑定echo
	if err = c.Validate(sfd); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: errs,
		})
	}

	errs = service.ResendExportFile(sfd.Files, dc, sfd.InWatchPath, sfd.CustomPath)

	// success
	return c.JSON(http.StatusOK, &model.ResponseError{
		Status: model.SUCCESS,
		Errors: errs,
	})
}
