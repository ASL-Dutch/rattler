package util

const (
	SUCCESS = "success"
	FAIL    = "fail"
)

type (
	HttpResponse struct {
		// Status success, fail
		Status string `json:"status"`
		// Error messages
		Message string `json:"message"`
		// Response data
		Data interface{} `json:"data"`
	}

	ResponseError struct {
		// Status success, fail
		Status string `json:"status"`
		// Error messages
		Errors []string `json:"errors"`
	}
)
