package scoring

import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
)

// CalculateScore 实现指定的评分机制
// 具体评分规则:
// 1. 正则匹配得1分
// 2. YARA匹配得1分
// 3. 正则和YARA同时匹配额外加2分
// 4. callable为true且融合预测模型置信度>0.85时加2分
// 5. 文本统计特征异常且callable为true时加2分
// 6. 最高分限制为5分
func CalculateScore(findings []*types.Finding, featureSet *features.FeatureSet) types.RiskLevel {
	if findings == nil || len(findings) == 0 {
		return types.RiskNone
	}

	var totalScore int = 0

	// 跟踪匹配状态
	hasRegexMatch := false
	hasYaraMatch := false
	highConfidencePrediction := false
	hasStatisticalAnomaly := false

	// 1. 分析各检测器结果
	for _, finding := range findings {
		switch finding.AnalyzerName {
		case "regex":
			hasRegexMatch = true
			logging.InfoLogger.Printf("检测到正则匹配")

		case "yara":
			hasYaraMatch = true
			logging.InfoLogger.Printf("检测到YARA匹配")

		case "svm_prosses":
			if finding.Confidence > 0.91 {
				highConfidencePrediction = true
				logging.InfoLogger.Printf("检测到高置信度融合模型预测: %.4f", finding.Confidence)
			}
		case "statistical":
			// 检测到统计分析器的发现，表示统计特征异常
			hasStatisticalAnomaly = true
			logging.InfoLogger.Printf("检测到统计特征异常")
		}
	}

	// 2. 根据规则计算分数
	// 规则1: 正则匹配得1分
	if hasRegexMatch {
		totalScore += 1
		logging.InfoLogger.Printf("正则匹配加1分，当前总分: %d", totalScore)
	}

	// 规则2: YARA匹配得1分
	if hasYaraMatch {
		totalScore += 1
		logging.InfoLogger.Printf("YARA匹配加1分，当前总分: %d", totalScore)
	}

	// 规则3: 正则和YARA同时匹配额外加2分
	if hasRegexMatch && hasYaraMatch {
		totalScore += 2
		logging.InfoLogger.Printf("正则和YARA同时匹配额外加2分，当前总分: %d", totalScore)
	}

	// 规则4: callable为true且高置信度预测时加2分
	hasCallable := featureSet != nil && featureSet.Callable
	// if hasCallable {
	// 	logging.InfoLogger.Printf("检测到可执行关键函数(callable=true)")
	// }

	if hasCallable && highConfidencePrediction {
		totalScore += 2
		logging.InfoLogger.Printf("可执行关键函数+高置信度预测加2分，当前总分: %d", totalScore)
	}

	// 规则5(新增): 统计特征异常且callable为true时加2分
	if hasCallable && hasStatisticalAnomaly {
		totalScore += 2
		logging.InfoLogger.Printf("可执行关键函数+统计特征异常加2分，当前总分: %d", totalScore)
	}

	// 规则6: 最高分限制为5分
	if totalScore > 5 {
		logging.InfoLogger.Printf("当前分数(%d)超过上限，调整为5分", totalScore)
		totalScore = 5
	}

	// 3. 将分数转换为风险等级
	var riskLevel types.RiskLevel
	switch {
	case totalScore >= 5:
		riskLevel = types.RiskCritical
	case totalScore >= 4:
		riskLevel = types.RiskHigh
	case totalScore >= 3:
		riskLevel = types.RiskMedium
	case totalScore >= 1:
		riskLevel = types.RiskLow
	default:
		riskLevel = types.RiskNone
	}

	logging.InfoLogger.Printf("最终评分: %d，风险等级: %s", totalScore, riskLevel.String())
	return riskLevel
}
