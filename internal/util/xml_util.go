package util

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"regexp"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"

	log "github.com/sirupsen/logrus"
)

// 不同编码到Go编码转换器的映射
var encodingMap = map[string]encoding.Encoding{
	"ISO8859-1":    charmap.ISO8859_1,
	"ISO-8859-1":   charmap.ISO8859_1,
	"Windows-1252": charmap.Windows1252,
}

// CharsetReader 返回一个从指定字符集转换为UTF-8的Reader
func CharsetReader(charset string, input io.Reader) (io.Reader, error) {
	// 统一转为大写并去除特殊字符，便于匹配
	charset = strings.ToUpper(charset)
	charset = strings.Replace(charset, "-", "", -1)

	if enc, ok := encodingMap[charset]; ok {
		return transform.NewReader(input, enc.NewDecoder()), nil
	}

	// 如果没有找到匹配的编码，返回原始reader（假设是UTF-8）
	log.Warnf("未知字符集: %s，尝试直接处理", charset)
	return input, nil
}

// CompressXML 压缩XML文档内容，去除不必要的空白字符
// 但保持XML文档的完整性与正确性
func CompressXML(xmlContent string) string {
	// 如果内容为空，直接返回
	if len(xmlContent) == 0 {
		return xmlContent
	}

	// 第一种方法：使用正则表达式去除XML标签之间的空白
	// 去除XML标签间的空白字符，但保留CDATA和引号内的内容
	re := regexp.MustCompile(`>\s+<`)
	compressed := re.ReplaceAllString(xmlContent, "><")

	// 第二种方法：使用XML解析和重写的方式
	// 如果正则表达式方法失败，尝试使用XML解析方法
	if !isValidXML(compressed) {
		// 尝试解析和重写XML内容
		var buf bytes.Buffer
		decoder := xml.NewDecoder(strings.NewReader(xmlContent))
		// 设置字符集Reader
		decoder.CharsetReader = CharsetReader

		encoder := xml.NewEncoder(&buf)

		// 禁用encoder的缩进功能，确保紧凑输出
		encoder.Indent("", "")

		for {
			token, err := decoder.Token()
			if err != nil {
				// 到达文件结尾或解析出错
				break
			}
			err = encoder.EncodeToken(token)
			if err != nil {
				log.Warnf("XML压缩过程中出现编码错误: %v", err)
				// 如果出错，返回原始内容
				return xmlContent
			}
			// XML文档结束
			if _, ok := token.(xml.EndElement); ok {
				err = encoder.Flush()
				if err != nil {
					log.Warnf("XML压缩过程中出现刷新错误: %v", err)
					return xmlContent
				}
			}
		}

		if isValidXML(buf.String()) {
			return buf.String()
		}
	}

	// 如果第二种方法也失败，返回第一种方法的结果或原始内容
	if isValidXML(compressed) {
		return compressed
	}

	// 如果所有方法都失败，返回原始内容
	return xmlContent
}

// AdvancedCompressXML 对XML进行更强力的压缩，应用多种压缩技术
// 包括去除注释、重写属性格式等
func AdvancedCompressXML(xmlContent string) string {
	// 首先尝试基本压缩
	compressed := CompressXML(xmlContent)

	// 去除XML注释 <!-- comment -->
	commentRe := regexp.MustCompile(`<!--[\s\S]*?-->`)
	withoutComments := commentRe.ReplaceAllString(compressed, "")

	// 确保注释删除后仍然是有效的XML
	if isValidXML(withoutComments) {
		compressed = withoutComments
	}

	// 去除XML声明的不必要空格
	declRe := regexp.MustCompile(`<\?xml(\s+[^>]*?)\?>`)
	withoutDeclSpaces := declRe.ReplaceAllStringFunc(compressed, func(match string) string {
		// 保留属性但去除多余空格
		attrRe := regexp.MustCompile(`\s+([a-zA-Z0-9_\-:]+)=["']([^"']*)["']`)
		return attrRe.ReplaceAllString(match, ` $1="$2"`)
	})

	// 确保声明压缩后仍然是有效的XML
	if isValidXML(withoutDeclSpaces) {
		compressed = withoutDeclSpaces
	}

	// 压缩属性间的空格
	attrSpaceRe := regexp.MustCompile(`\s{2,}`)
	withCompressedAttrSpaces := attrSpaceRe.ReplaceAllString(compressed, " ")

	// 确保属性空格压缩后仍然是有效的XML
	if isValidXML(withCompressedAttrSpaces) {
		compressed = withCompressedAttrSpaces
	}

	return compressed
}

// StreamingCompressXML 流式处理大型XML文件
// 此方法适用于处理大型XML文件，不会一次性将整个文件加载到内存中
func StreamingCompressXML(reader io.Reader, writer io.Writer) error {
	decoder := xml.NewDecoder(reader)
	// 设置字符集Reader处理非UTF-8编码
	decoder.CharsetReader = CharsetReader

	encoder := xml.NewEncoder(writer)

	// 禁用缩进以获得最紧凑的输出
	encoder.Indent("", "")

	var err error
	var token xml.Token

	for {
		// 获取下一个XML标记
		token, err = decoder.Token()
		if err != nil {
			if err == io.EOF {
				// 文件结束，不是错误
				err = nil
			}
			break
		}

		// 特殊处理某些类型的标记
		switch t := token.(type) {
		case xml.CharData:
			// 处理字符数据
			// 检查是否为CDATA (由golang xml包统一处理为CharData)
			data := string(t)
			isCDATA := strings.HasPrefix(data, "<![CDATA[") && strings.HasSuffix(data, "]]>")

			if isCDATA {
				// CDATA部分保持原样
				err = encoder.EncodeToken(t)
			} else {
				// 删除普通文本中的多余空白
				trimmed := bytes.TrimSpace(t)
				if len(trimmed) > 0 {
					err = encoder.EncodeToken(xml.CharData(trimmed))
				}
			}
		case xml.Comment:
			// 可选：跳过注释
			continue
		case xml.ProcInst:
			// 处理处理指令（如 <?xml version="1.0"?>）
			err = encoder.EncodeToken(t)
		case xml.StartElement:
			// 处理开始标签
			err = encoder.EncodeToken(t)
		case xml.EndElement:
			// 处理结束标签
			err = encoder.EncodeToken(t)
		case xml.Directive:
			// 处理指令（如 <!DOCTYPE ...>）
			err = encoder.EncodeToken(t)
		default:
			// 其他类型的标记直接编码
			err = encoder.EncodeToken(t)
		}

		if err != nil {
			break
		}
	}

	if err == nil {
		// 刷新所有挂起的输出
		err = encoder.Flush()
	}

	return err
}

// isValidXML 检查XML字符串是否有效
func isValidXML(xmlContent string) bool {
	// 创建一个解码器，并设置字符集reader
	decoder := xml.NewDecoder(strings.NewReader(xmlContent))
	decoder.CharsetReader = CharsetReader

	// 使用解码器解析整个文档
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			// 到达文档末尾，文档有效
			return true
		}
		if err != nil {
			// 解析出错，文档无效
			return false
		}
	}
}

// CompressXMLFile 读取XML文件并将压缩后的内容写入字符串
// 适用于处理较大的XML文件
func CompressXMLFile(filePath string) (string, error) {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// 用于存储压缩后的XML内容
	var buf bytes.Buffer

	// 使用流式压缩方法处理文件
	err = StreamingCompressXML(file, &buf)
	if err != nil {
		return "", err
	}

	// 检查压缩后的内容是否有效
	result := buf.String()
	if !isValidXML(result) {
		// 如果流式压缩后XML无效，尝试使用更安全的方法
		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			return "", err
		}

		// 使用高级压缩方法
		return AdvancedCompressXML(string(fileContent)), nil
	}

	return result, nil
}
