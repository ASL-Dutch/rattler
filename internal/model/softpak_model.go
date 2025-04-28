package model

type (
	SearchFileResult struct {
		// Type TAX_BILL, EXPORT_XML
		Type       string `json:"type"`
		SearchText string `json:"searchText"`
		Filename   string `json:"filename"`
		Filepath   string `json:"filepath"`
	}

	// ExportFileListDTO Export 文件列表DTO
	ExportFileListDTO struct {
		// Type TAX_BILL, EXPORT_XML
		Filename string `json:"filename"`
		Filepath string `json:"filepath"`
		// 文件大小 bytes
		Size int64 `json:"size"`
		// 修改时间
		ModTime string `json:"modifiedTime"`
	}
)
