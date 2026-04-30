package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"sysafari.com/softpak/rattler/internal/config"
	"sysafari.com/softpak/rattler/internal/util"
)

const (
	TaxBillInfoType  = "TAX_BILL_INFO"
	ReportStatusType = "REPORT_STATUS"
)

// TaxBillExportInfo is the MQ payload for parsed tax-bill information.
type TaxBillExportInfo struct {
	// 标识字段：表示当前为税金文件信息的MQ消息
	// 如果没有此字段则表示为报关状态通知消息
	// TAX_BILL_INFO: 税金文件信息; REPORT_STATUS: 报关状态通知消息;
	Type           string                   `json:"type"`
	FileName       string                   `json:"fileName"`
	DeclareCountry string                   `json:"declareCountry"`
	TaxBill        *util.TaxPdfMededelingen `json:"taxBill"`
}

// SendTaxBillInfoToExportMQ waits for PDF readiness, parses tax-bill info and publishes to export MQ.
func SendTaxBillInfoToExportMQ(pdfPath, declareCountry string) error {
	declareCountry = strings.ToUpper(strings.TrimSpace(declareCountry))
	info, err := readStableTaxBillInfo(pdfPath)
	if err != nil {
		return err
	}

	payload := TaxBillExportInfo{
		Type:           TaxBillInfoType,
		FileName:       info.FileName,
		DeclareCountry: declareCountry,
		TaxBill:        info,
	}

	bf := bytes.NewBuffer(nil)
	enc := json.NewEncoder(bf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("serialize tax-bill info failed: %w", err)
	}

	publishMessageToMQ(bf.String(), declareCountry)
	log.Infof("税金单信息已发布到Export MQ: file=%s, country=%s", info.FileName, declareCountry)
	return nil
}

func readStableTaxBillInfo(pdfPath string) (*util.TaxPdfMededelingen, error) {
	maxAttempts := config.GlobalConfig.Watchers.Pdf.TaxInfoPublish.MaxAttempts
	checkIntervalMs := config.GlobalConfig.Watchers.Pdf.TaxInfoPublish.CheckIntervalMs
	minContentSize := config.GlobalConfig.Watchers.Pdf.TaxInfoPublish.MinContentSize

	// defaults
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	if checkIntervalMs <= 0 {
		checkIntervalMs = 1500
	}
	if minContentSize <= 0 {
		minContentSize = 1024
	}

	checkInterval := time.Duration(checkIntervalMs) * time.Millisecond
	var (
		lastSize int64 = -1
		stable   int
		lastErr  error
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		st, err := os.Stat(pdfPath)
		if err != nil {
			lastErr = err
			time.Sleep(checkInterval)
			continue
		}
		if st.IsDir() {
			return nil, fmt.Errorf("path is directory: %s", pdfPath)
		}
		if st.Size() < minContentSize {
			lastErr = fmt.Errorf("pdf content too small: %d", st.Size())
			time.Sleep(checkInterval)
			continue
		}

		if st.Size() == lastSize {
			stable++
		} else {
			stable = 0
			lastSize = st.Size()
		}
		// require two consecutive same size observations
		if stable < 1 {
			time.Sleep(checkInterval)
			continue
		}

		info, err := util.ParseTaxPdfMededelingen(pdfPath)
		if err != nil {
			lastErr = err
			log.Debugf("税金单解析失败，等待重试(%d/%d): %s, err=%v", attempt, maxAttempts, pdfPath, err)
			time.Sleep(checkInterval)
			continue
		}
		return info, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("tax-bill pdf was not ready after retries")
	}
	return nil, fmt.Errorf("读取并解析税金单失败: %s, err=%w", pdfPath, lastErr)
}
