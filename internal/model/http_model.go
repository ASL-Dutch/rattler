package model

import (
	"github.com/go-playground/validator"
	"github.com/labstack/echo/v4"
	"net/http"
)

type (
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
		// InListeningPath 是否是监听路径中的文件
		InListeningPath bool `json:"inListeningPath" validate:"required"`

		// FilePaths 需要重新发送的文件名
		FilePaths []string `json:"filePaths" validate:"required"`
	}
)

func (v *CustomValidator) Validate(i interface{}) error {
	if err := v.Validator.Struct(i); err != nil {
		// Optionally, you could return the error to give each route more control over the status code
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return nil
}
