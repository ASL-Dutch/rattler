package util

import (
	"path/filepath"
	"regexp"
	"strings"
)

// taxBillBackupPrefixRe 备份后文件名前缀：yyyyMM_（如 202605_DI-18-AI-2026-00441622.pdf）。
var taxBillBackupPrefixRe = regexp.MustCompile(`^(20\d{2})(0[1-9]|1[0-2])_(.+)$`)

// TaxBillProcessableMarker 需解析 Mededelingen 并发布 MQ 的税金单文件名标记（basename 须包含此段）。
// 支持标准命名（如 DI-18-AI-2026-00441622.pdf）及备份重放前缀（如 202605_DI-18-AI-2026-00441622.pdf）。
// 不含此标记的 PDF 仍会备份，但不解析内容。
const TaxBillProcessableMarker = "DI-18"

// IsProcessableTaxBillFileName 判断是否应解析税金单内容并发布 MQ（不含 DI-18 的文件仅备份）。
func IsProcessableTaxBillFileName(filePathOrName string) bool {
	base := filepath.Base(strings.TrimSpace(filePathOrName))
	return base != "" && strings.Contains(base, TaxBillProcessableMarker)
}

// HasTaxBillBackupNamePrefix 判断文件名是否为备份命名（含 yyyyMM_ 前缀）。
func HasTaxBillBackupNamePrefix(filePathOrName string) bool {
	_, _, ok := ParseTaxBillBackupPrefix(filePathOrName)
	return ok
}

// ParseTaxBillBackupPrefix 从备份文件名解析归档年月，供 backupDir/yyyy/mm/ 定位。
func ParseTaxBillBackupPrefix(filePathOrName string) (year, month string, ok bool) {
	base := filepath.Base(strings.TrimSpace(filePathOrName))
	m := taxBillBackupPrefixRe.FindStringSubmatch(base)
	if len(m) != 4 {
		return "", "", false
	}
	return m[1], m[2], true
}

// NormalizeTaxBillFileName 规范为 basename，并统一 pdf 扩展名为小写 .pdf。
func NormalizeTaxBillFileName(filePathOrName string) string {
	base := filepath.Base(strings.TrimSpace(filePathOrName))
	if base == "" {
		return base
	}
	ext := filepath.Ext(base)
	if ext == "" {
		return base + ".pdf"
	}
	stem := strings.TrimSuffix(base, ext)
	return stem + ".pdf"
}
