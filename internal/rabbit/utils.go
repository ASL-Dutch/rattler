package rabbit

import (
	"time"
)

// ParseDuration 解析字符串格式的时间间隔，如果解析失败则返回默认值
func ParseDuration(durationStr string, defaultDuration time.Duration) time.Duration {
	if durationStr == "" {
		return defaultDuration
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return defaultDuration
	}

	return duration
}
