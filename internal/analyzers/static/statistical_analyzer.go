// path: internal/analyzers/static/statistical_analyzer.go
package static

import (
	"bt-shieldml/internal/features" // Import features package
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"fmt"
	"math"
)

// StatisticalThresholds 保存阈值，使用 features 包中的类型
type StatisticalThresholds struct {
	MinStat features.StatisticalFeatures `json:"MinStat"`
	MaxStat features.StatisticalFeatures `json:"MaxStat"`
}

// StatisticalAnalyzer 为统计检查实现了 engine.Analyzer 接口。
type StatisticalAnalyzer struct {
	thresholds StatisticalThresholds
}

/**
 * @Description: 返回默认阈值
 * @author: Mr wpl
 * @return StatisticalThresholds 默认阈值
 */
func GetDefaultStatisticalThresholds() StatisticalThresholds {
	minStat := features.StatisticalFeatures{
		LM: math.NaN(), LVC: 0.1, WM: math.NaN(), WVC: math.NaN(),
		SR: 10.0, TR: math.NaN(), SPL: 0.001, IE: math.NaN(),
	}
	maxStat := features.StatisticalFeatures{
		LM: 2048.0, LVC: math.NaN(), WM: 1024.0, WVC: math.NaN(),
		SR: math.NaN(), TR: math.NaN(), SPL: math.NaN(), IE: math.NaN(),
	}
	return StatisticalThresholds{MinStat: minStat, MaxStat: maxStat}
}

/**
 * @Description: 创建一个新的分析器并设置阈值。
 * @author: Mr wpl
 * @return *StatisticalAnalyzer 新的分析器
 * @return error 错误信息
 */
func NewStatisticalAnalyzer() (*StatisticalAnalyzer, error) {
	defaultThresholds := GetDefaultStatisticalThresholds()

	return &StatisticalAnalyzer{
		thresholds: defaultThresholds,
	}, nil
}

/**
 * @Description: 返回分析器的名称。
 * @author: Mr wpl
 * @return string 分析器的名称
 */
func (a *StatisticalAnalyzer) Name() string {
	return "statistical"
}

/**
 * @Description: 返回此分析器所需的特征。
 * @author: Mr wpl
 * @return []string 分析器所需的特征
 */
func (a *StatisticalAnalyzer) RequiredFeatures() []string {
	// Needs the calculated statistical features and the callable flag from the FeatureSet
	return []string{"statistical", "callable"}
}

/**
 * @Description: 执行统计分析。
 * @author: Mr wpl
 * @param fileInfo 文件信息
 * @param content 文件内容
 * @param featureSet 特征集
 * @return *types.Finding 发现
 */
func (a *StatisticalAnalyzer) Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) {
	// 1. Check if required features are present in the FeatureSet
	if featureSet == nil || featureSet.Statistical == nil {
		if len(content) == 0 {
			// It's okay for empty files to not have stats
			// logging.InfoLogger.Printf("Skipping statistical analysis for empty file: %s", fileInfo.Path)
			return nil, nil
		}
		// Log an error if stats are missing for non-empty content
		logging.ErrorLogger.Printf("Required 'statistical' feature missing in FeatureSet for %s", fileInfo.Path)
		return nil, fmt.Errorf("missing statistical features")
	}

	// 2. Perform the check using the abnormality helper and the callable flag
	calculatedStats := featureSet.Statistical
	isStatAbnormal := IsStatisticalAbnormal(calculatedStats, a.thresholds) // Use helper
	isAstCallable := featureSet.Callable

	// 3. Create finding only if both conditions are met
	if isStatAbnormal && isAstCallable {
		desc := fmt.Sprintf("文件存在统计特征异常且存在可执行代码结构 (e.g., LM:%.0f, LVC:%.4f, WM:%.0f, WVC:%.2f, SR:%.2f, IE:%.4f)",
			calculatedStats.LM, calculatedStats.LVC, calculatedStats.WM, calculatedStats.WVC, calculatedStats.SR, calculatedStats.IE)

		return &types.Finding{
			AnalyzerName: a.Name(),
			Description:  desc,
			Risk:         types.RiskMedium, // Assign risk level as per requirement
			Confidence:   0.7,              // Example confidence
		}, nil
	}

	return nil, nil // No finding
}

/**
 * @Description: 检查统计特征是否异常。
 * @author: Mr wpl
 * @param sf 统计特征
 * @param std 阈值
 * @return bool 是否异常
 */
func IsStatisticalAbnormal(sf *features.StatisticalFeatures, std StatisticalThresholds) bool {
	if sf == nil {
		return false
	}
	return outOfRange(sf.LM, std.MinStat.LM, std.MaxStat.LM) ||
		outOfRange(sf.LVC, std.MinStat.LVC, std.MaxStat.LVC) ||
		outOfRange(sf.WM, std.MinStat.WM, std.MaxStat.WM) ||
		outOfRange(sf.WVC, std.MinStat.WVC, std.MaxStat.WVC) ||
		outOfRange(sf.SR, std.MinStat.SR, std.MaxStat.SR) ||
		outOfRange(sf.TR, std.MinStat.TR, std.MaxStat.TR) ||
		outOfRange(sf.SPL, std.MinStat.SPL, std.MaxStat.SPL) ||
		outOfRange(sf.IE, std.MinStat.IE, std.MaxStat.IE)
}

/**
 * @Description: 检查值是否在最小/最大范围之外。
 * @author: Mr wpl
 * @param x 值
 * @param min 最小值
 * @param max 最大值
 * @return bool 是否异常
 */
func outOfRange(x float64, min float64, max float64) bool {
	// Check less than min, ignoring NaN comparison
	if !math.IsNaN(min) && x < min {
		return true
	}
	// Check greater than max, ignoring NaN comparison
	if !math.IsNaN(max) && x > max {
		return true
	}
	return false
}
