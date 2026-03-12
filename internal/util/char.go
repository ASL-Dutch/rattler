// 字符串处理工具包
package util

import (
	"fmt"
	"strings"
	"time"
)

// IsDatePrefix 判断字符串是否为日期前缀
// s: 字符串, 如： 202301_52912_DI-2023-112_NEW.xml
// delimiter 分隔符, 如： _
// format: 日期格式格式, 如: "200601"
func IsDatePrefix(s, delimiter, format string) bool {
	if s == "" || delimiter == "" || format == "" {
		return false
	}

	parts := strings.SplitN(s, delimiter, 2)
	if len(parts) < 1 {
		return false
	}

	prefix := parts[0]
	_, err := time.Parse(format, prefix)

	if err != nil {
		fmt.Printf("IsDatePrefix 错误: %v\n", err)
	}

	return true
}

// DateToPathFormat 将日期字符串转换为路径格式
// dateStr: 日期字符串
// inputFormat: 输入的日期格式, 如: "200601"
// level: 路径层级, 1=年, 2=年/月, 3=年/月/日, 0=根据日期格式自动生成
// 返回: 路径格式字符串, 如: "2006/01/02"
func DateToPathFormat(dateStr, inputFormat string, level int) (string, error) {
	if dateStr == "" || inputFormat == "" {
		return "", fmt.Errorf("日期字符串或格式不能为空")
	}

	// 解析日期
	t, err := time.Parse(inputFormat, dateStr)
	if err != nil {
		return "", fmt.Errorf("日期格式解析错误: %v", err)
	}

	// 根据输入格式自动决定层级
	if level == 0 {
		if strings.Contains(inputFormat, "02") {
			level = 3 // 包含日，生成年/月/日
		} else if strings.Contains(inputFormat, "01") {
			level = 2 // 包含月，生成年/月
		} else {
			level = 1 // 只包含年，只生成年
		}
	}

	// 根据层级返回不同格式
	switch level {
	case 1:
		return fmt.Sprintf("%04d", t.Year()), nil
	case 2:
		return fmt.Sprintf("%04d/%02d", t.Year(), t.Month()), nil
	case 3:
		return fmt.Sprintf("%04d/%02d/%02d", t.Year(), t.Month(), t.Day()), nil
	default:
		return "", fmt.Errorf("不支持的路径层级: %d", level)
	}
}
