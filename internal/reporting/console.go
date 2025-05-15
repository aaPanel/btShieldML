/*
 * @Author: wpl
 * @Date: 2025-04-15 10:37:04
 * @Description: 终端命令行输出日志
 */
package reporting

import (
	"bt-shieldml/pkg/types"
	"fmt"
	"os"
	"sort"
)

type ConsoleReporter struct{}

/**
 * @Description: 创建新的终端命令行输出日志
 * @author: Mr wpl
 * @return *ConsoleReporter: 终端命令行输出日志
 */
func NewConsoleReporter() *ConsoleReporter {
	return &ConsoleReporter{}
}

/**
 * @Description: 生成终端命令行输出日志
 * @author: Mr wpl
 * @param results []*types.ScanResult: 扫描结果
 * @param outputPath string: 输出路径
 */
func (r *ConsoleReporter) Generate(results []*types.ScanResult, outputPath string) error {
	if outputPath != "" {
		fmt.Fprintf(os.Stderr, "Warning: Console reporter does not support output path '%s'. Printing to stdout.\n", outputPath)
	}

	// Sort results by path for consistent output
	sort.Slice(results, func(i, j int) bool {
		return results[i].File.Path < results[j].File.Path
	})

	fmt.Println("\n--- Scan Report ---")
	riskCounts := make(map[types.RiskLevel]int)
	var totalFiles, errorFiles int

	for _, res := range results {
		totalFiles++
		if res.Error != nil {
			fmt.Printf("[ERROR] %s : %v\n", res.File.Path, res.Error)
			riskCounts[types.RiskUnknown]++
			errorFiles++
			continue
		}

		riskCounts[res.OverallRisk]++

		// Print details only for files with findings or risk > None
		if res.OverallRisk > types.RiskNone || len(res.Findings) > 0 {
			fmt.Printf("[%s] %s (Risk: %s, Time: %s)\n", res.OverallRisk.String(), res.File.Path, res.OverallRisk.String(), res.Duration)
			if len(res.Findings) > 0 {
				// Sort findings by risk level (descending)
				sort.Slice(res.Findings, func(i, j int) bool {
					return res.Findings[i].Risk > res.Findings[j].Risk
				})
				for _, f := range res.Findings {
					fmt.Printf("  -> [%s] %s: %s\n", f.Risk.String(), f.AnalyzerName, f.Description)
				}
			}
			if res.SkippedAST {
				fmt.Println("  -> AST analysis skipped due to early high-risk finding.")
			}
		}
	}

	fmt.Println("\n--- Summary ---")
	fmt.Printf("Total Files Scanned: %d\n", totalFiles)
	fmt.Printf("Files with Errors:   %d\n", errorFiles)
	fmt.Printf("Risk Levels Found:\n")
	// Print counts for each risk level
	levels := []types.RiskLevel{types.RiskCritical, types.RiskHigh, types.RiskMedium, types.RiskLow, types.RiskNone, types.RiskUnknown}
	for _, level := range levels {
		if count, ok := riskCounts[level]; ok && count > 0 {
			fmt.Printf("  - %-8s : %d\n", level.String(), count)
		}
	}
	fmt.Println("--- End Report ---")

	return nil
}
