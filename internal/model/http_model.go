package model

import (
	"net/http"

	"github.com/go-playground/validator"
	"github.com/labstack/echo/v4"
)

const (
	SUCCESS = "success"
	FAIL    = "fail"
)

type (
	// HttpResponse 通用响应结构体
	HttpResponse struct {
		// Status success, fail
		Status string `json:"status"`
		// Error messages
		Message string `json:"message"`
		// Response data
		Data interface{} `json:"data"`
	}

	// ResponseError 通用错误响应结构体
	ResponseError struct {
		// Status success, fail
		Status string `json:"status"`
		// Error messages
		Errors []string `json:"errors"`
	}

	CustomValidator struct {
		Validator *validator.Validate
	}

	SearchFileRequest struct {
		// DeclareCountry NL, BE
		DeclareCountry string `json:"declareCountry" validate:"required"`
		// Year exp: 2022
		Year string `json:"year"`
		// Month exp: 09
		Month string `json:"month"`
		// Type TAX_BILL, EXPORT_XML
		Type string `json:"type" validate:"required"`
		// Filenames Support use Job number
		Filenames []string `json:"filenames" validate:"required"`
	}

	FileResendRequest struct {
		// InWatchPath 是否是监听路径中（默认：false,即在备份路径中）
		InWatchPath bool `json:"inWatchPath"`

		// CustomPath 自定义路径(默认：不需要, 特殊情况，用户可能需要指当前需要重新触发监听的export xml 文件所在的路径)
		CustomPath string `json:"customPath"`
		// Files 需要重新发送的文件名，多个文件名需要在同一路径中。监听路径或者备份路径
		Files []string `json:"files" validate:"required"`
	}
)

func (v *CustomValidator) Validate(i interface{}) error {
	if err := v.Validator.Struct(i); err != nil {
		// Optionally, you could return the error to give each route more control over the status code
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}
