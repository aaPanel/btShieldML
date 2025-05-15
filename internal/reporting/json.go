package reporting

import (
	"bt-shieldml/pkg/types"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// 简化版扫描结果
type SimpleResult struct {
	Filename string `json:"filename"`
	Type     string `json:"type"`
	Risk     int    `json:"risk"`        // 原始风险等级（数字）
	RiskText string `json:"risk_text"`   // 风险等级描述
	Desc     string `json:"description"` // 简短描述
}

// JsonReporter 实现 Reporter 接口
type JsonReporter struct{}

/**
 * @Description: 创建新的JSON报告
 * @author: Mr wpl
 * @return *JsonReporter: JSON报告
 */
func NewJsonReporter() *JsonReporter {
	return &JsonReporter{}
}

/**
 * @Description: 生成JSON报告
 * @author: Mr wpl
 * @param results []*types.ScanResult: 扫描结果
 * @param outputPath string: 输出路径
 * @return error: 错误
 */
func (r *JsonReporter) Generate(results []*types.ScanResult, outputPath string) error {
	// 确保输出固定到 data/webshellJson.json
	if outputPath == "" {
		// 首先确保data目录存在
		dataDir := "data"
		if _, err := os.Stat(dataDir); os.IsNotExist(err) {
			if err := os.MkdirAll(dataDir, 0755); err != nil {
				return err
			}
		}
		outputPath = filepath.Join(dataDir, "webshellJson.json")
	}

	// 创建简化版结果
	simplified := make([]SimpleResult, 0, len(results))

	for _, res := range results {
		if res.Error != nil {
			continue
		}

		// 提取文件类型
		fileType := strings.TrimPrefix(strings.ToLower(filepath.Ext(res.File.Path)), ".")

		// 风险级别描述
		var riskText string
		var desc string
		var riskScore int

		// 明确处理所有风险级别
		switch res.OverallRisk {
		case types.RiskNone:
			riskText = "正常"
			desc = "未发现问题"
			riskScore = 0 // 确保RiskNone映射为0
		case types.RiskLow:
			riskText = "疑似木马"
			desc = "检测到可疑特征"
			riskScore = 1
		case types.RiskMedium:
			riskText = "疑似木马"
			desc = "检测到可疑特征"
			riskScore = 3
		case types.RiskHigh:
			riskText = "疑似木马"
			desc = "检测到可疑特征"
			riskScore = 4
		case types.RiskCritical:
			riskText = "木马文件"
			desc = "检测为高危木马"
			riskScore = 5
		default:
			riskText = "未知"
			desc = "检测过程异常"
			riskScore = 0
		}

		// 添加到简化结果中
		simplified = append(simplified, SimpleResult{
			Filename: filepath.Base(res.File.Path),
			Type:     fileType,
			Risk:     riskScore, // 使用明确映射的分数
			RiskText: riskText,
			Desc:     desc,
		})
	}

	// 创建或打开输出文件
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// 使用map包装，和前端约定好格式
	finalResult := map[string]interface{}{
		"results": simplified,
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(finalResult)
}
