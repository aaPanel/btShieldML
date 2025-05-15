/*
 * @Date: 2025-04-15 10:46:05
 * @Editors: Mr wpl
 * @Description: yara匹配
 */
package static

import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/embedded"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hillu/go-yara/v4"
)

type YaraAnalyzer struct {
	analyzerName string // Renamed field
	rules        *yara.Rules
}

/**
 * @Description: 创建yara分析器
 * @author: Mr wpl
 * @param dataPath 数据路径
 * @return *YaraAnalyzer yara分析器
 * @return error 错误
 */
func NewYaraAnalyzer(dataPath string) (*YaraAnalyzer, error) {
	// 尝试从嵌入文件加载
	ruleData, err := embedded.GetFileContent("data/signatures/Webshells_rules.yar")
	if err != nil {
		logging.WarnLogger.Printf("未找到嵌入的YARA规则，尝试从磁盘加载: %v", err)
		// 继续使用原来的磁盘加载逻辑
		ruleFilePath := filepath.Join(dataPath, "Webshells_rules.yar")

		if _, err := os.Stat(ruleFilePath); os.IsNotExist(err) {
			logging.WarnLogger.Printf("YARA rule file not found at %s: %v. YARA analyzer will be inactive.", ruleFilePath, err)
			return &YaraAnalyzer{analyzerName: "yara", rules: nil}, nil // Use renamed field
		}

		compiler, err := yara.NewCompiler()
		if err != nil {
			return nil, fmt.Errorf("failed to create yara compiler: %w", err)
		}

		file, err := os.Open(ruleFilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open yara rule file %s: %w", ruleFilePath, err)
		}
		defer file.Close()

		err = compiler.AddFile(file, "webshell")
		if err != nil {
			return nil, fmt.Errorf("failed to add yara rule file %s to compiler: %w", ruleFilePath, err)
		}

		rules, err := compiler.GetRules()
		if err != nil {
			return nil, fmt.Errorf("failed to compile yara rules from %s: %w", ruleFilePath, err)
		}

		return &YaraAnalyzer{analyzerName: "yara", rules: rules}, nil
	}

	// 使用嵌入的规则数据
	compiler, err := yara.NewCompiler()
	if err != nil {
		return nil, fmt.Errorf("创建yara编译器失败: %w", err)
	}

	err = compiler.AddString(string(ruleData), "webshell")
	if err != nil {
		return nil, fmt.Errorf("添加yara规则到编译器失败: %w", err)
	}

	rules, err := compiler.GetRules()
	if err != nil {
		return nil, fmt.Errorf("编译yara规则失败: %w", err)
	}
	// logging.InfoLogger.Printf("成功编译嵌入的YARA规则")

	return &YaraAnalyzer{analyzerName: "yara", rules: rules}, nil
}

/**
 * @Description: 返回分析器名称
 * @author: Mr wpl
 * @return string 分析器名称
 */
func (a *YaraAnalyzer) Name() string {
	return a.analyzerName
}

/**
 * @Description: 返回分析器所需的特征
 * @author: Mr wpl
 * @return []string 分析器所需的特征
 */
func (a *YaraAnalyzer) RequiredFeatures() []string {
	return nil
}

/**
 * @Description: 分析文件，是否匹配yara规则
 * @author: Mr wpl
 * @param fileInfo 文件信息
 * @param content 文件内容
 * @param featureSet 特征集
 * @return *types.Finding 发现
 */
func (a *YaraAnalyzer) Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) {
	if a.rules == nil {
		return nil, nil
	}

	scanner, err := yara.NewScanner(a.rules)
	if err != nil {
		logging.ErrorLogger.Printf("Failed to create YARA scanner for %s: %v", fileInfo.Path, err)
		return nil, fmt.Errorf("yara scanner creation failed: %w", err)
	}

	var matches yara.MatchRules
	err = scanner.SetCallback(&matches).ScanMem(content)
	if err != nil {
		logging.WarnLogger.Printf("YARA scan failed for %s: %v", fileInfo.Path, err)
		return nil, fmt.Errorf("yara scan execution failed: %w", err)
	}

	if len(matches) > 0 {
		match := matches[0]
		logging.InfoLogger.Printf("YARA match found for %s (Rule: %s)", fileInfo.Path, match.Rule)
		return &types.Finding{
			AnalyzerName: a.analyzerName, // Use renamed field
			Description:  fmt.Sprintf("Matched YARA rule: %s", match.Rule),
			Risk:         types.RiskCritical,
			Confidence:   1.0,
		}, nil
	}

	return nil, nil
}
