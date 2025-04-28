package web

import (
	"github.com/go-playground/validator"
	"github.com/labstack/echo/v4"
	log "github.com/sirupsen/logrus"
	echoSwagger "github.com/swaggo/echo-swagger"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/model"
	"sysafari.com/softpak/rattler/internal/web/handler"
)

// StartServer initializes and starts the Echo web server
// @title Rattler API
// @version 1.0
// @description This is a server for soft-pak, can download tax-bill and export xml files
// @termsOfService http://swagger.io/terms/
// @contact.name Joker
// @contact.email ljr@y-clouds.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:7003
func StartServer() {
	e := echo.New()
	e.Validator = &model.CustomValidator{
		Validator: validator.New(),
	}

	// Register routes
	registerRoutes(e)

	// Start server
	port := config.GlobalConfig.GetPort()

	log.Infof("Rattler server started on port %s", port)
	log.Error(e.Start(":" + port))
}

// registerRoutes registers all routes to the Echo instance
func registerRoutes(e *echo.Echo) {
	// Swagger documentation
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	// File download routes
	e.GET("/download/pdf/:origin/:target", handler.DownloadTaxPdf)
	e.GET("/download/xml/:dc/:filename", handler.DownloadExportXml)

	// File search route
	e.POST("/search/file", handler.SearchFile)

	// Export routes
	e.GET("/export/list/:dc", handler.ExportListenFiles)
	e.POST("/export/remover/:dc", handler.ExportFileResend)
}
