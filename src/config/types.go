package config

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ByteSize 字节大小，兼容两种 TOML 写法：
//
//	fileSize = 2147483648  （裸整数，单位字节）
//	fileSize = "2GB"       （带单位字符串，支持 B/KB/MB/GB/TB 十进制与 KiB/MiB/GiB/TiB 二进制）
//
// go-toml v2 对所有标量（含裸整数）都会调用 TextUnmarshaler 并传入原始文本，
// 因此两种写法可由同一解析入口统一处理。
type ByteSize int64

var byteSizeUnits = map[string]float64{
	"b":   1,
	"kb":  1e3,
	"mb":  1e6,
	"gb":  1e9,
	"tb":  1e12,
	"kib": 1 << 10,
	"mib": 1 << 20,
	"gib": 1 << 30,
	"tib": 1 << 40,
}

// UnmarshalText 实现 encoding.TextUnmarshaler。
func (b *ByteSize) UnmarshalText(text []byte) error {
	v, err := parseByteSize(string(text))
	if err != nil {
		return err
	}
	*b = ByteSize(v)
	return nil
}

// String 实现 fmt.Stringer，按二进制单位输出人类可读大小（如 1.50 GiB）。
func (b ByteSize) String() string {
	const unit = 1 << 10
	if b < unit {
		return fmt.Sprintf("%d B", int64(b))
	}
	div, exp := int64(unit), 0
	for n := int64(b) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("字节大小不能为空")
	}
	i := 0
	for i < len(s) && (s[i] >= '0' && s[i] <= '9' || s[i] == '.') {
		i++
	}
	numStr, unitStr := s[:i], strings.ToLower(strings.TrimSpace(s[i:]))
	if numStr == "" {
		return 0, fmt.Errorf("无法解析字节大小 %q", s)
	}
	// 无单位时默认为字节（与裸整数写法一致）
	mult := 1.0
	if unitStr != "" {
		var ok bool
		if mult, ok = byteSizeUnits[unitStr]; !ok {
			return 0, fmt.Errorf("未知的字节大小单位 %q（支持 B/KB/MB/GB/TB 或 KiB/MiB/GiB/TiB）", unitStr)
		}
	}
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil || num < 0 {
		return 0, fmt.Errorf("无法解析字节大小 %q", s)
	}
	v := num * mult
	if v > math.MaxInt64 {
		return 0, fmt.Errorf("字节大小 %q 超出范围", s)
	}
	return int64(v), nil
}

// Duration 时长，TOML 字符串写法（如 "20m"、"1h30m"、"90s"）。
// 加载期即完成解析与校验，避免旧版在运行期解析失败后静默回退默认值的问题。
type Duration time.Duration

// UnmarshalText 实现 encoding.TextUnmarshaler。
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(strings.TrimSpace(string(text)))
	if err != nil {
		return fmt.Errorf("无法解析时长 %q（示例：20m、1h30m、90s）", string(text))
	}
	*d = Duration(v)
	return nil
}
