package util

import (
	"bytes"
	"encoding/json"
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

// TaxPdfMededelingen 税金单 PDF 解析结果（荷兰税单 Mededelingen 版式及 MQ 载荷）。
// JSON 字段名与 jobNo 推导由 MarshalJSON 统一处理，勿依赖外部序列化工具函数。
type TaxPdfMededelingen struct {
	FileName       string // basename，含扩展名
	JobNo          string // 可选；为空时 MarshalJSON 从 FileName 自动推导
	ParseSuccess   bool   // Mededelingen 等业务字段是否解析成功
	FailureReason  string // 解析失败时的原因说明
	HandlingHint   string // 解析失败时的处理建议
	MessageType    string // 如 CCTAXA
	ContactOffice  string // 如 NL000074
	BankAccount    string // IBAN
	Reference      string // 付款参考号
	AmountRaw      string // 欧洲小数格式原文
	AmountStandard string // 标准小数点金额
	RawSectionText string // Mededelingen 原始片段
}

// taxPdfMededelingenJSON MQ 载荷 JSON 形态（小写驼峰）。
type taxPdfMededelingenJSON struct {
	FileName       string `json:"fileName"`
	JobNo          string `json:"jobNo"`
	ParseSuccess   bool   `json:"parseSuccess"`
	FailureReason  string `json:"failureReason,omitempty"`
	HandlingHint   string `json:"handlingHint,omitempty"`
	MessageType    string `json:"messageType,omitempty"`
	ContactOffice  string `json:"contactOffice,omitempty"`
	BankAccount    string `json:"bankAccount,omitempty"`
	Reference      string `json:"reference,omitempty"`
	AmountRaw      string `json:"amountRaw,omitempty"`
	AmountStandard string `json:"amountStandard,omitempty"`
	RawSectionText string `json:"rawSectionText,omitempty"`
}

// MarshalJSON 序列化为小写驼峰 JSON；jobNo 在输出时由 fileName 自动推导（若 JobNo 未显式设置）。
func (t *TaxPdfMededelingen) MarshalJSON() ([]byte, error) {
	if t == nil {
		return []byte("null"), nil
	}
	payload := taxPdfMededelingenJSON{
		FileName:       t.FileName,
		JobNo:          t.resolvedJobNo(),
		ParseSuccess:   t.ParseSuccess,
		FailureReason:  t.FailureReason,
		HandlingHint:   t.HandlingHint,
	}
	if t.ParseSuccess {
		payload.MessageType = t.MessageType
		payload.ContactOffice = t.ContactOffice
		payload.BankAccount = t.BankAccount
		payload.Reference = t.Reference
		payload.AmountRaw = t.AmountRaw
		payload.AmountStandard = t.AmountStandard
		payload.RawSectionText = t.RawSectionText
	}
	return json.Marshal(payload)
}

// ResolvedJobNo 返回用于日志或业务展示的 jobNo（与 MarshalJSON 规则一致）。
func (t *TaxPdfMededelingen) ResolvedJobNo() string {
	if t == nil {
		return ""
	}
	return t.resolvedJobNo()
}

func (t *TaxPdfMededelingen) resolvedJobNo() string {
	if strings.TrimSpace(t.JobNo) != "" {
		return strings.TrimSpace(t.JobNo)
	}
	return jobNoFromFileName(t.FileName)
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
// JobNo 始终从文件名解析，不依赖 PDF 正文。
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
	out := &TaxPdfMededelingen{
		FileName: fileName,
	}

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

	out.RawSectionText = strings.TrimSpace(block)
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

	out.ParseSuccess = true
	return out, nil
}

// jobNoFromFileName 从税金单文件名提取 Job No：取 basename（去扩展名）中最后一个 '-' 之后的段，并去掉高位补零。
// 例如 DI-08-AI-2026-00440559.pdf → 00440559 → 440559。
func jobNoFromFileName(fileName string) string {
	base := fileName
	if ext := filepath.Ext(fileName); ext != "" {
		base = strings.TrimSuffix(fileName, ext)
	}
	idx := strings.LastIndex(base, "-")
	if idx < 0 || idx >= len(base)-1 {
		if digitsOnly(base) {
			return trimLeadingZerosJobSegment(base)
		}
		return base
	}
	return trimLeadingZerosJobSegment(base[idx+1:])
}

func trimLeadingZerosJobSegment(segment string) string {
	trimmed := strings.TrimLeft(strings.TrimSpace(segment), "0")
	if trimmed == "" {
		return "0"
	}
	return trimmed
}

// MededelingenFailureReason 未找到 Mededelingen 区域时的标准原因说明。
const MededelingenFailureReason = "PDF 正文中未找到 Mededelingen 区域，或无法从 PDF 提取该段文本"

// MededelingenHandlingHint 未找到 Mededelingen 时的处理建议。
const MededelingenHandlingHint = "请确认：(1) 文件已在监听目录完整生成（建议打开 PDF 查看末页是否含 Mededelingen 与 EINDE）；" +
	"(2) 文件为荷兰标准税金单版式，而非运输单等其他 PDF；(3) 扫描件或无文本层的 PDF 无法自动解析，需人工补录或重新打印；" +
	"(4) 若文件刚生成可等待片刻由系统重试，或修正源文件后重新放入监听目录"

// PDFReadFailureReason PDF 无法打开或不是有效 PDF 时的原因说明。
const PDFReadFailureReason = "无法打开或读取 PDF 文件（文件损坏、尚未写完或非 PDF 格式）"

// PDFReadHandlingHint PDF 读取失败时的处理建议。
const PDFReadHandlingHint = "请确认文件已完整写入监听目录且可正常用 PDF 阅读器打开；检查磁盘与权限；必要时重新导出税金单"

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
