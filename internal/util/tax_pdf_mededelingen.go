package util

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/ledongthuc/pdf"
)

// TaxPdfMededelingen 税金单 PDF 底部「Mededelingen」区块中的结构化字段（荷兰税单常见版式）。
type TaxPdfMededelingen struct {
	FileName        string // 税金单 PDF 文件名（filepath.Base(pdfPath)，含扩展名）
	CustomsId       string // 报关/关务标识：取自 FileName 按下划线 _ 分割后的第一段（非 PDF 内容）
	MessageType     string // 消息类型，如 CCTAXA
	ContactOffice   string // 联系税务机关代码，如 NL000074
	BankAccount     string // IBAN
	Reference       string // 付款参考号
	AmountRaw       string // 金额原文（欧洲小数格式，如 609,22）
	AmountStandard  string // AmountRaw 转为小数点格式（无千分位），如 609.22；解析失败时为空
	RawSectionText  string // 从「Mededelingen」到「EINDE」之间的原始文本，便于核对与调试
}

var (
	// ErrMededelingenNotFound PDF 中未找到 Mededelingen 区块
	ErrMededelingenNotFound = errors.New("pdf: Mededelingen section not found")
	// ErrNotPDF 文件不是可读 PDF
	ErrNotPDF = errors.New("pdf: not a valid PDF file")

	reBankrekeningNL = regexp.MustCompile(`(?i)NL[0-9]{2}[A-Z]{4}[0-9]{10}`)
	reReferentie     = regexp.MustCompile(`(?i)Referentie:\s*([0-9][0-9\s]{8,40})`)
	reBedrag         = regexp.MustCompile(`(?i)Bedrag:\s*([0-9]{1,3}(?:[.,][0-9]{2})|[0-9]{1,3}(?:\.[0-9]{3})*(?:[.,][0-9]{2}))`)
)

// ParseTaxPdfMededelingen 从税金单 PDF 文件路径解析底部 Mededelingen 区域。
// 优先通过文本提取：自最后一页向前扫描包含「Mededelingen」的页面，避免整本大文档全量拼接。
//
// FileName 为 pdfPath 的 basename（含扩展名）；CustomsId 不来自 PDF 正文，由 FileName 按 '_' 取首段，
// 规则见 CustomsId 字段注释。例如 BSIU9634507_18_1777524400792.PDF → FileName 同全名，CustomsId 为 BSIU9634507；
// 若文件名中无 '_'，则 CustomsId 等于 FileName。
func ParseTaxPdfMededelingen(pdfPath string) (*TaxPdfMededelingen, error) {
	if pdfPath == "" {
		return nil, fmt.Errorf("pdf: empty path")
	}
	st, err := os.Stat(pdfPath)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("pdf: path is a directory: %s", pdfPath)
	}

	fileName := filepath.Base(pdfPath)
	customsID := customsIDFromFileName(fileName)

	r, err := openPDFReaderAtPath(pdfPath)
	if err != nil {
		return nil, err
	}

	pageText, err := findMededelingenPageText(r)
	if err != nil {
		return nil, err
	}

	block := extractMededelingenBlock(pageText)
	if block == "" {
		return nil, ErrMededelingenNotFound
	}

	out := &TaxPdfMededelingen{
		FileName:        fileName,
		CustomsId:       customsID,
		RawSectionText:  strings.TrimSpace(block),
	}
	out.MessageType = valueAfterLabelUntil(block, "Bericht:", "Contact", "Bankrekening")
	out.ContactOffice = valueAfterLabelUntil(block, "Contact kantoor:", "Bankrekening", "Referentie")
	if ib := reBankrekeningNL.FindString(block); ib != "" {
		out.BankAccount = strings.ToUpper(ib)
	}
	if m := reReferentie.FindStringSubmatch(block); len(m) > 1 {
		out.Reference = strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "")
	}
	applySubmatch(reBedrag, block, &out.AmountRaw)
	if std, err := EuropeanAmountToStandard(out.AmountRaw); err == nil {
		out.AmountStandard = std
	}

	return out, nil
}

// customsIDFromFileName 从税金单文件名（basename）提取 CustomsId：按下划线 _ 分割取第一段。
func customsIDFromFileName(fileName string) string {
	if i := strings.IndexByte(fileName, '_'); i >= 0 {
		return fileName[:i]
	}
	return fileName
}

// EuropeanAmountToStandard 将荷兰/欧洲常见金额字符串转为标准小数点形式（无千分位分隔符）。
// 支持逗号为小数位：如 "609,22"、"1.234,56"；若已为 "609.22" 且小数部分最多两位也可识别。
func EuropeanAmountToStandard(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, " ", ""))
	if raw == "" {
		return "", fmt.Errorf("amount: empty")
	}

	lastComma := strings.LastIndex(raw, ",")
	lastDot := strings.LastIndex(raw, ".")

	// 逗号为小数点、点为千分位（荷兰税单常见）
	if lastComma >= 0 && (lastDot < 0 || lastComma > lastDot) {
		intPart := strings.ReplaceAll(raw[:lastComma], ".", "")
		fracPart := raw[lastComma+1:]
		return joinIntFracStandard(intPart, fracPart)
	}

	// 已为美式小数点且仅含数字与点
	if lastDot >= 0 && lastComma < 0 {
		intPart := strings.ReplaceAll(raw[:lastDot], ",", "")
		fracPart := raw[lastDot+1:]
		return joinIntFracStandard(intPart, fracPart)
	}

	if digitsOnly(raw) {
		return raw + ".00", nil
	}

	return "", fmt.Errorf("amount: unsupported format %q", raw)
}

func joinIntFracStandard(intPart, fracPart string) (string, error) {
	intPart = strings.TrimSpace(intPart)
	fracPart = strings.TrimSpace(fracPart)
	if intPart == "" || intPart == "-" {
		intPart = "0"
	}
	neg := strings.HasPrefix(intPart, "-")
	if neg {
		intPart = strings.TrimPrefix(intPart, "-")
	}
	if !digitsOnly(intPart) || !digitsOnly(fracPart) {
		return "", fmt.Errorf("amount: non-digit parts int=%q frac=%q", intPart, fracPart)
	}
	if len(fracPart) > 2 {
		return "", fmt.Errorf("amount: fraction too long %q", fracPart)
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return "", err
	}
	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil || frac >= 100 {
		return "", fmt.Errorf("amount: bad fraction")
	}
	cents := whole*100 + frac
	if neg {
		cents = -cents
	}
	sign := ""
	cc := cents
	if cc < 0 {
		sign = "-"
		cc = -cc
	}
	return fmt.Sprintf("%s%d.%02d", sign, cc/100, cc%100), nil
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// openPDFReaderAtPath 打开 PDF。部分税金单生成器在版本号后输出空格再换行，
// ledongthuc/pdf 的头部校验要求版本位后紧跟 CR/LF，因此在内存中规范化首部字节。
func openPDFReaderAtPath(pdfPath string) (*pdf.Reader, error) {
	raw, err := os.ReadFile(pdfPath)
	if err != nil {
		return nil, err
	}
	raw = fixPDFHeaderForReader(raw)
	raw = fixTrailerStartxrefForReader(raw)
	br := bytes.NewReader(raw)
	return pdf.NewReader(br, int64(len(raw)))
}

func fixPDFHeaderForReader(b []byte) []byte {
	if len(b) < 9 || !bytes.HasPrefix(b, []byte("%PDF-1.")) {
		return b
	}
	if b[7] < '0' || b[7] > '7' {
		return b
	}
	if b[8] == '\r' || b[8] == '\n' {
		return b
	}
	j := 8
	for j < len(b) && (b[j] == ' ' || b[j] == '\t') {
		j++
	}
	if j >= len(b) {
		return b
	}
	out := make([]byte, 0, len(b))
	out = append(out, b[:8]...)
	out = append(out, '\n')
	out = append(out, b[j:]...)
	return out
}

// fixTrailerStartxrefForReader 修正尾部 startxref 行：部分生成器在关键字与换行之间插入空格，
// ledongthuc/pdf 的 findLastLine 要求关键字后紧跟换行。仅在文件末尾 8KiB 内替换，避免误伤内容流。
func fixTrailerStartxrefForReader(b []byte) []byte {
	const tail = 8192
	if len(b) <= tail {
		return applyStartxrefLineFixes(b)
	}
	prefix := b[:len(b)-tail]
	suffix := append([]byte(nil), b[len(b)-tail:]...)
	suffix = applyStartxrefLineFixes(suffix)
	out := make([]byte, 0, len(prefix)+len(suffix))
	out = append(out, prefix...)
	out = append(out, suffix...)
	return out
}

func applyStartxrefLineFixes(s []byte) []byte {
	s = bytes.ReplaceAll(s, []byte("startxref \n"), []byte("startxref\n"))
	s = bytes.ReplaceAll(s, []byte("startxref \r\n"), []byte("startxref\r\n"))
	s = bytes.ReplaceAll(s, []byte("startxref \r"), []byte("startxref\r"))
	return s
}

func applySubmatch(re *regexp.Regexp, s string, dest *string) {
	if m := re.FindStringSubmatch(s); len(m) > 1 {
		*dest = strings.TrimSpace(m[1])
	}
}

// valueAfterLabelUntil 在 block 中取 label 之后的值，直到任一 stop 子串（大小写不敏感）出现。
// 用于 PDF 文本流常把相邻字段连在一起（无空格）的情况。
func valueAfterLabelUntil(block, label string, stop ...string) string {
	li := indexFold(block, label)
	if li < 0 {
		return ""
	}
	start := li + len(label)
	rest := strings.TrimSpace(block[start:])
	if rest == "" {
		return ""
	}
	end := len(rest)
	for _, s := range stop {
		si := indexFold(rest, s)
		if si >= 0 && si < end {
			end = si
		}
	}
	return strings.TrimSpace(rest[:end])
}

func indexFold(hay, needle string) int {
	return strings.Index(strings.ToLower(hay), strings.ToLower(needle))
}

// findMededelingenPageText 从最后一页起向前查找包含 Mededelingen 的页面文本。
func findMededelingenPageText(r *pdf.Reader) (string, error) {
	n := r.NumPage()
	if n < 1 {
		return "", ErrNotPDF
	}
	const marker = "Mededelingen"
	for i := n; i >= 1; i-- {
		p := r.Page(i)
		txt, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		if strings.Contains(txt, marker) {
			return txt, nil
		}
	}
	// 个别 PDF 分页导致关键词跨页：回退为全文流（仍是一次解码，仅多页时略慢）
	pr, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	all, err := io.ReadAll(pr)
	if err != nil {
		return "", err
	}
	s := string(all)
	if strings.Contains(s, marker) {
		return s, nil
	}
	return "", ErrMededelingenNotFound
}

// extractMededelingenBlock 截取从标题 Mededelingen 到 EINDE（含常见变体）之间的片段。
func extractMededelingenBlock(page string) string {
	const startTok = "Mededelingen"
	idx := strings.Index(page, startTok)
	if idx < 0 {
		return ""
	}
	rest := page[idx:]
	end := len(rest)
	for _, tok := range []string{"\nEINDE", "\r\nEINDE", "EINDE"} {
		if j := strings.Index(rest, tok); j >= 0 && j < end {
			end = j
		}
	}
	return strings.TrimSpace(rest[:end])
}
