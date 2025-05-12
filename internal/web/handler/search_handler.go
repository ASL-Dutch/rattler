package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/service"
)

// SearchFile Search for tax bill files and Export declaration XML files
// @Summary      搜索文件
// @Description  可检索税金单文件以及export报关结果文件，可使用文件名部分做模糊匹配。建议使用Job number 进行检索
// @Tags         search
// @Accept       json
// @Produce      json
// @Param        message  body  model.SearchFileRequest  true  "检索内容"
// @Success      200 {object} []model.SearchFileResult
// @Failure      400 {object} model.ResponseError
// @Failure      404 {object} model.ResponseError
// @Failure      500 {object} model.ResponseError
// @Router       /search/file [post]
func SearchFile(c echo.Context) (err error) {
	var errs []string
	sfd := new(model.SearchFileRequest)
	if err = c.Bind(sfd); err != nil {
		errs = append(errs, err.Error())
	}
	if err = c.Validate(sfd); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: errs,
		})
	}

	sf := &service.SearchFile{
		DeclareCountry: sfd.DeclareCountry,
		Year:           sfd.Year,
		Month:          sfd.Month,
		Type:           sfd.Type,
		Filenames:      sfd.Filenames,
	}
	files, errs := sf.GetSearchResult()
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, &model.ResponseError{
			Status: model.FAIL,
			Errors: errs,
		})
	}

	// success
	return c.JSON(http.StatusOK, files)
}
