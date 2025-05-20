/*
 * @Author: wpl
 * @Date: 2025-04-15 09:44:28
 * @LastEditors: Please set LastEditors
 * @LastEditTime: 2025-05-15 18:06:42
 * @Description: 通用类型定义
 */
package types

import "time"

// 定义检测到的风险级别
type RiskLevel int

const (
	RiskUnknown  RiskLevel = iota // 0: Error or unable to determine
	RiskNone                      // 1: No risk detected / Benign / Whitelisted
	RiskLow                       // 2: Low risk / Suspicious pattern
	RiskMedium                    // 3: Medium risk / Likely malicious pattern
	RiskHigh                      // 4: High risk / Strong malicious indicators
	RiskCritical                  // 5: Critical risk / Confirmed malicious / Known bad hash/YARA rule match
)

// 返回风险级别的字符串表示
func (rl RiskLevel) String() string {
	switch rl {
	case RiskNone:
		return "Safe"
	case RiskLow:
		return "Low"
	case RiskMedium:
		return "Medium"
	case RiskHigh:
		return "High"
	case RiskCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// DataPaths 定义数据文件路径
type DataPaths struct {
	Models     string `yaml:"models"`
	Signatures string `yaml:"signatures"`
	Config     string `yaml:"config"`
	Rules      string `yaml:"rules"` // Added for explicit rule path
}

// Performance 定义性能相关配置
type Performance struct {
	Concurrency int `yaml:"concurrency"`
}

// 文件信息结构体,保存文件的基本信息
type FileInfo struct {
	Path     string
	Size     int64
	ModTime  time.Time
	MIMEType string // Optional: Can be added later
	// Content []byte - Avoid storing full content here for memory efficiency
}

// Finding represents a specific finding by an analyzer.
type Finding struct {
	AnalyzerName string    // Name of the analyzer that generated this finding
	Description  string    // Description of the finding (e.g., "Matched Hash", "YARA Rule: XYZ")
	Risk         RiskLevel // Assessed risk level by this analyzer
	Confidence   float64   // Confidence score (0.0 to 1.0, optional for static)
	// Snippet      string    // Relevant code snippet (optional)
	// LineNumber   int       // Line number (optional)
}

// ScanResult holds the overall result for a single scanned file.
// 保存单个扫描文件的总体结果
type ScanResult struct {
	File        FileInfo      // Information about the scanned file
	OverallRisk RiskLevel     // Final aggregated risk level
	Findings    []*Finding    // List of findings from different analyzers
	Error       error         // Any error encountered during scanning this file
	Duration    time.Duration // Time taken to scan this file
	SkippedAST  bool          // Flag if AST generation was skipped due to early high-risk finding
}

// Output 定义输出相关配置
type Output struct {
	Format string `yaml:"format"` // console, json, html
}

// Config structure (基本示例,根据需要扩展)
type Config struct {
	DataPaths        DataPaths   `yaml:"data_paths"`
	Performance      Performance `yaml:"performance"`
	Output           Output      `yaml:"output"`
	EnabledAnalyzers []string    `yaml:"enabled_analyzers"` // List of analyzer names to run
	// Add more config options: Exclusions, ScanDepth etc.
}
