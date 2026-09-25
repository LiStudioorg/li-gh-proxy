package config

import (
	"bytes"
	"errors"

	"github.com/pelletier/go-toml/v2"
)

// utf8BOM Windows 记事本等编辑器保存 UTF-8 时会带 BOM，解析前剥离以保证兼容
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// decodeTOML 将配置文件内容解码进 cfg（调用前 cfg 须已初始化为默认值），返回非致命警告。
//
// 策略：先用严格模式（DisallowUnknownFields）探测未知字段——仅告警不阻断，
// 避免拼写误差或版本升级导致服务无法启动；其余解析/类型错误原样返回。
func decodeTOML(cfg *AppConfig, data []byte) ([]string, error) {
	data = bytes.TrimPrefix(data, utf8BOM)
	probe := DefaultConfig()
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(probe); err != nil {
		var strictErr *toml.StrictMissingError
		if !errors.As(err, &strictErr) {
			return nil, err
		}
		warnings := make([]string, 0, len(strictErr.Errors))
		for i := range strictErr.Errors {
			warnings = append(warnings, "配置文件包含未识别的配置项，已忽略: "+strictErr.Errors[i].String())
		}
		if err := toml.Unmarshal(data, cfg); err != nil {
			return warnings, err
		}
		return warnings, nil
	}

	*cfg = *probe
	return nil, nil
}
