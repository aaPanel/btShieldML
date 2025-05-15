package static

import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var highRiskRegexList []*regexp.Regexp
var regexCompileOnce sync.Once
var regexCompileErr error

/**
 * @Description: 初始化正则表达式规则
 * @author: Mr wpl
 */
func initializeRegexRules() {
	regexCompileOnce.Do(func() {
		// logging.InfoLogger.Println("Compiling regex rules...")
		// Rules provided, adapted slightly for Go's regex engine if needed
		rules := []string{
			`(?i)@\$\_=`,
			`(?i)eval\s*\(\s*(['"])\s*\?>`,
			`(?i)eval\s*\(\s*gzinflate\s*\(`,
			`(?i)eval\s*\(\s*str_rot13\s*\(`,
			`(?i)base64_decode\s*\(\s*\$\_`,
			`(?i)eval\s*\(\s*gzuncompress\s*\(`,
			`(?i)assert\s*\(\s*(['"]|\s*)\s*\$`,
			`(?i)(require_once|include_once|require|include)\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)gzinflate\s*\(\s*base64_decode\s*\(`,
			`(?i)echo\s*\(\s*file_get_contents\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)c99shell`, `(?i)cmd\.php`,
			`(?i)call_user_func\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)str_rot13`,
			`(?i)webshell`, `(?i)EgY_SpIdEr`, `(?i)SECFORCE`,
			`(?i)eval\s*\(\s*base64_decode\s*\(`,
			`(?i)array_map\s*\(.{1,25}(eval|assert|ass(?-i:\\\\x65)rt).{1,25}\$_(GET|POST|REQUEST)`,
			`(?i)call_user_func\s*\(.{0,30}\$_(GET|POST|REQUEST)`,
			`(?i)gzencode`,
			`(?i)call_user_func\s*\(\s*("|\')assert("|\')`,
			`(?i)fputs\s*\(\s*fopen\s*\(\s*(.+)\s*,\s*(['"])w(['"])\s*\)\s*,\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)\s*\[`,
			`(?i)file_put_contents\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)\s*\[[^\]]+\]\s*,\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)\$_(POST|GET|REQUEST|COOKIE)\s*\[[^\]]+\]\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)\s*\[`,
			`(?i)assert\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)eval\s*\(\s*(['"]|\s*)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)base64_decode\s*\(\s*gzuncompress\s*\(`,
			`(?i)gzuncompress\s*\(\s*base64_decode\s*\(`,
			`(?i)eval\s*\(\s*gzdecode\s*\(`,
			`(?i)preg_replace\s*\(\s*["']/.*["']\s*,\s*["'].*["']\s*,\s*.*\s*\)\s*;/si`,
			`(?i)Scanners`, `(?i)phpspy`, `(?i)cha88\.cn`,
			`(?i)chr\s*\(\s*\d+\s*\)\s*\.\s*chr\s*\(\s*\d+\s*\)`,
			`(?i)\$\_\s*=\s*\$\_`,
			`(?i)\$\w+\s*\(\s*\$\{`,
			`(?i)\(array\)\s*\$_(POST|GET|REQUEST|COOKIE)`,
			`(?i)\$\w+\s*\(\s*["']/.*["']\s*,\s*["'].*/e["']`,
			`(?i)("e"|"E")\s*\.\s*("v"|"V")\s*\.\s*("a"|"A")\s*\.\s*("l"|"L")`,
			`(?i)('e'|'E')\s*\.\s*('v'|'V')\s*\.\s*('a'|'A')\s*\.\s*('l'|'L')`,
			`(?i)@\s*preg_replace\s*\(\s*["']/.*["']/e\s*,\s*\$_POST\s*\[`,
			`(?i)\$\{\s*'_'`,
			`(?i)@\s*\$\_\s*\(\s*\$\_`,
		}

		highRiskRegexList = make([]*regexp.Regexp, 0, len(rules))
		var compileErrors []string
		for _, rule := range rules {
			re, err := regexp.Compile(rule)
			if err != nil {
				compileErrors = append(compileErrors, fmt.Sprintf("Rule '%s': %v", rule, err))
				continue
			}
			highRiskRegexList = append(highRiskRegexList, re)
		}

		if len(compileErrors) > 0 {
			regexCompileErr = fmt.Errorf("failed to compile %d regex rules: %s", len(compileErrors), strings.Join(compileErrors, "; "))
			logging.ErrorLogger.Printf("Regex Compilation Errors: %v", regexCompileErr)
		}
	})
}

/**
 * @Description: 正则表达式分析器
 * @author: Mr wpl
 */
type RegexAnalyzer struct {
	analyzerName string // Renamed field
}

/**
 * @Description: 创建RegexAnalyzer实例
 * @author: Mr wpl
 * @return *RegexAnalyzer 正则表达式分析器实例
 * @return error 错误信息
 */
func NewRegexAnalyzer() (*RegexAnalyzer, error) {
	initializeRegexRules()
	if regexCompileErr != nil && len(highRiskRegexList) == 0 {
		return nil, fmt.Errorf("regex analyzer failed to initialize: no rules compiled: %w", regexCompileErr)
	} else if regexCompileErr != nil {
		logging.WarnLogger.Printf("Regex analyzer initialized with %d rules, but some failed to compile: %v", len(highRiskRegexList), regexCompileErr)
	}
	return &RegexAnalyzer{analyzerName: "regex"}, nil // Use renamed field
}

/**
 * @Description: 返回分析器名称
 * @author: Mr wpl
 * @return string 分析器名称
 */
func (a *RegexAnalyzer) Name() string {
	return a.analyzerName // Return the value of the renamed field
}

/**
 * @Description: 返回分析器所需的特征
 * @author: Mr wpl
 * @return []string 分析器所需的特征
 */
func (a *RegexAnalyzer) RequiredFeatures() []string {
	return nil
}

/**
 * @Description: 分析文件
 * @author: Mr wpl
 * @param fileInfo 文件信息
 * @param content 文件内容
 * @param featureSet 特征集
 * @return *types.Finding 发现
 */
func (a *RegexAnalyzer) Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) {
	if len(highRiskRegexList) == 0 {
		return nil, nil
	}

	for _, re := range highRiskRegexList {
		if re.Match(content) {
			logging.InfoLogger.Printf("Regex match found for %s (Rule: %s)", fileInfo.Path, re.String())
			return &types.Finding{
				AnalyzerName: a.analyzerName,
				Description:  fmt.Sprintf("Matched high-risk regex pattern: %s", re.String()),
				Risk:         types.RiskCritical,
				Confidence:   0.9,
			}, nil
		}
	}

	return nil, nil
}
