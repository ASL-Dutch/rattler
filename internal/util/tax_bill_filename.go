package util

import (
	"path/filepath"
	"strings"
)

// TaxBillProcessableMarker 税金单监听仅处理文件名（basename）包含此段的 PDF。
// 支持标准命名（如 DI-18-AI-2026-00441622.pdf）及备份重放前缀（如 202605_DI-18-AI-2026-00441622.pdf）。
const TaxBillProcessableMarker = "DI-18"

// IsProcessableTaxBillFileName 判断税金单是否应进入业务处理（发布 MQ、备份等）。
func IsProcessableTaxBillFileName(filePathOrName string) bool {
	base := filepath.Base(strings.TrimSpace(filePathOrName))
	return base != "" && strings.Contains(base, TaxBillProcessableMarker)
}
