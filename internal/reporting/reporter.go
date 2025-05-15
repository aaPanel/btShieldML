/*
 * @Date: 2025-04-15 16:17:39
 * @Editors: Mr wpl
 * @Description: 报告生成器接口定义
 */
package reporting

import "bt-shieldml/pkg/types"

// Reporter 定义了报告生成器的通用接口
type Reporter interface {
	// Generate 根据扫描结果生成报告，并写入到 outputPath
	// 如果报告类型是直接输出（如控制台），outputPath 可能会被忽略
	Generate(results []*types.ScanResult, outputPath string) error
}
