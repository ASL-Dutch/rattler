package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
// Mededelingen 解析失败时仍发布消息（含 fileName、jobNo 及失败说明），以便下游感知并继续备份流程。
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
	if info.ParseSuccess {
		log.Infof("税金单信息已发布到Export MQ: file=%s, jobNo=%s, country=%s", info.FileName, info.ResolvedJobNo(), declareCountry)
	} else {
		log.Warnf("税金单解析不完整仍发布MQ: file=%s, jobNo=%s, country=%s, reason=%s",
			info.FileName, info.ResolvedJobNo(), declareCountry, info.FailureReason)
	}
	return nil
}

func readStableTaxBillInfo(pdfPath string) (*util.TaxPdfMededelingen, error) {
	maxAttempts := config.GlobalConfig.Watchers.Pdf.TaxInfoPublish.MaxAttempts
	checkIntervalMs := config.GlobalConfig.Watchers.Pdf.TaxInfoPublish.CheckIntervalMs
	minContentSize := config.GlobalConfig.Watchers.Pdf.TaxInfoPublish.MinContentSize

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
			stable = 1
			lastSize = st.Size()
		}
		const stabilityRequiredHits = 2
		if stable < stabilityRequiredHits {
			time.Sleep(checkInterval)
			continue
		}

		info, err := util.ParseTaxPdfMededelingen(pdfPath)
		if err == nil {
			return info, nil
		}
		lastErr = err
		if !isRetriableTaxBillParseErr(err) {
			return taxBillInfoAfterParseFailure(pdfPath, err), nil
		}
		log.Debugf("税金单解析失败，等待重试(%d/%d): %s, err=%v", attempt, maxAttempts, pdfPath, err)
		time.Sleep(checkInterval)
	}

	if lastErr != nil && isRetriableTaxBillParseErr(lastErr) {
		return taxBillInfoAfterParseFailure(pdfPath, lastErr), nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("tax-bill pdf was not ready after retries")
	}
	return nil, fmt.Errorf("读取并解析税金单失败: %s, err=%w", pdfPath, lastErr)
}

func isRetriableTaxBillParseErr(err error) bool {
	return errors.Is(err, util.ErrMededelingenNotFound)
}

func taxBillInfoAfterParseFailure(pdfPath string, err error) *util.TaxPdfMededelingen {
	reason, hint := taxBillParseFailureMeta(err)
	return &util.TaxPdfMededelingen{
		FileName:      filepath.Base(pdfPath),
		ParseSuccess:  false,
		FailureReason: reason,
		HandlingHint:  hint,
	}
}

func taxBillParseFailureMeta(err error) (reason, hint string) {
	switch {
	case errors.Is(err, util.ErrMededelingenNotFound):
		return util.MededelingenFailureReason, util.MededelingenHandlingHint
	case errors.Is(err, util.ErrNotPDF):
		return util.PDFReadFailureReason, util.PDFReadHandlingHint
	default:
		return fmt.Sprintf("税金单 PDF 解析失败: %v", err), util.MededelingenHandlingHint
	}
}
