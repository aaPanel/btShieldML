package engine

import (
	"bt-shieldml/internal/analyzers/ml" // Import ML analyzers
	"bt-shieldml/internal/analyzers/static"
	"bt-shieldml/internal/ast"
	"bt-shieldml/internal/features"
	"bt-shieldml/internal/reporting"
	"bt-shieldml/internal/scoring"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Engine 协调扫描过程
type Engine struct {
	config     *types.Config
	analyzers  map[string]Analyzer
	astManager ast.ASTManager // 持有 AST 管理器实例
}

/**
 * @Description: 初始化检测引擎
 * @author: Mr wpl
 * @param cfg *types.Config: 配置
 * @return *Engine: 引擎
 */
func NewEngine(cfg *types.Config) (*Engine, error) {
	var astMgr ast.ASTManager
	var err error

	// 默认初始化 AST通道
	needsAST := false

	// 需要AST的分析器
	astRequiredBy := []string{"regex", "yara", "bayes_words", "statistical", "svm_prosses"} // Add more if needed
	enabledSet := make(map[string]bool)
	for _, name := range cfg.EnabledAnalyzers {
		enabledSet[strings.ToLower(name)] = true
	}
	for _, req := range astRequiredBy {
		if enabledSet[req] {
			needsAST = true
			break
		}
	}

	if needsAST {
		astMgr, err = ast.NewPhpAstManager()
		if err != nil {
			logging.ErrorLogger.Printf("Failed to initialize AST Manager (PHP bridge start failed): %v. AST-dependent analyzers will be inactive.", err)
			// Don't return error here, allow engine to continue without AST features
			astMgr = nil
		}
	} else {
		logging.InfoLogger.Println("No AST-dependent analyzers enabled, skipping AST Manager initialization.")
	}

	// Initialize and register analyzers based on config
	enabledAnalyzers := make(map[string]Analyzer)
	analyzerErrors := []string{}

	// Use the enabledSet for quick lookup
	for nameLower := range enabledSet {
		var analyzer Analyzer
		var initErr error

		// Check if analyzer requires AST and if manager is available
		requiresAST := false
		for _, req := range astRequiredBy {
			if nameLower == req {
				requiresAST = true
				break
			}
		}
		if requiresAST && astMgr == nil {
			// logging.WarnLogger.Printf("Skipping initialization of analyzer '%s': requires AST but AST Manager failed to initialize.", nameLower)
			continue
		}

		switch nameLower {
		case "regex":
			analyzer, initErr = static.NewRegexAnalyzer()
		case "yara":
			analyzer, initErr = static.NewYaraAnalyzer(cfg.DataPaths.Signatures)
		case "statistical":
			analyzer, initErr = static.NewStatisticalAnalyzer() // Already checks for AST manager internally if needed
		// case "svm_ops":
		// 	analyzer, initErr = ml.NewSvmOpsAnalyzer(cfg.DataPaths.Models, cfg.DataPaths.Config)
		case "bayes_words":
			analyzer, initErr = ml.NewBayesWordsAnalyzer(cfg.DataPaths.Models)
		case "svm_prosses":
			analyzer, initErr = ml.NewSvmProssesAnalyzer(cfg.DataPaths.Models)
		default:
			logging.WarnLogger.Printf("Unknown analyzer specified in config: %s", nameLower)
			continue
		}

		if initErr != nil {
			errMsg := fmt.Sprintf("Failed to initialize analyzer '%s': %v", nameLower, initErr)
			logging.ErrorLogger.Println(errMsg)
			analyzerErrors = append(analyzerErrors, errMsg)
		} else if analyzer != nil {
			enabledAnalyzers[nameLower] = analyzer // Store by lowercase name
			// logging.InfoLogger.Printf("Analyzer '%s' initialized successfully.", nameLower)
		}
	}

	if len(enabledAnalyzers) == 0 {
		errMsg := "No analyzers were enabled or successfully initialized."
		if len(analyzerErrors) > 0 {
			errMsg += " Errors: " + strings.Join(analyzerErrors, "; ")
		}
		// Decide if this is fatal. Return warning for now.
		logging.ErrorLogger.Println(errMsg)
		// return nil, fmt.Errorf(errMsg) // Uncomment if no analyzers is a fatal error
	}

	return &Engine{
		config:     cfg,
		analyzers:  enabledAnalyzers,
		astManager: astMgr, // Store potentially nil AST manager
	}, nil
}

/**
 * @Description: 根据任务定义执行扫描
 * @author: Mr wpl
 * @param task *Task: 任务
 * @return error: 错误
 */
func (e *Engine) Scan(task *Task) error {
	// Cleanup AST Manager if it was initialized
	if e.astManager != nil {
		defer func() {
			if err := e.astManager.Cleanup(); err != nil {
				logging.ErrorLogger.Printf("Error during AST Manager cleanup: %v", err)
			}
		}()
	}

	filesToScan, err := findFiles(task.Paths, task.Exclusions)
	if err != nil {
		return fmt.Errorf("error finding files to scan: %w", err)
	}
	if len(filesToScan) == 0 {
		logging.InfoLogger.Println("No files found to scan.")
		if task.ReportPath != "" {
			return e.generateReport([]*types.ScanResult{}, task)
		}
		return nil
	}

	results := make([]*types.ScanResult, 0, len(filesToScan))
	var wg sync.WaitGroup
	resultChan := make(chan *types.ScanResult, len(filesToScan))

	concurrency := e.config.Performance.Concurrency
	if concurrency <= 0 {
		concurrency = 4 // Default if invalid
	}
	sem := make(chan struct{}, concurrency)

	startTime := time.Now()

	for _, filePath := range filesToScan {
		// Basic check before goroutine
		if _, statErr := os.Stat(filePath); statErr != nil {
			logging.WarnLogger.Printf("Skipping file %s: %v", filePath, statErr)
			// Add a result indicating the error for this file
			results = append(results, &types.ScanResult{
				File:  types.FileInfo{Path: filePath},
				Error: fmt.Errorf("stat error: %w", statErr),
			})
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(fp string) {
			defer wg.Done()
			defer func() { <-sem }()
			// Pass the engine's astManager to scanFile
			result := e.scanFile(fp, e.astManager)
			resultChan <- result
		}(filePath)
	}

	wg.Wait()
	close(resultChan)

	for res := range resultChan {
		results = append(results, res)
	}

	totalDuration := time.Since(startTime)
	logging.InfoLogger.Printf("Scanning finished in %s", totalDuration)

	// Generate reports
	return e.generateReport(results, task)
}

/**
 * @Description: 处理文件，接收 astManager 实例，用于 AST 解析
 * @author: Mr wpl
 * @param filePath string: 文件路径
 * @param astMgr ast.ASTManager: AST 管理器实例
 * @return *types.ScanResult: 扫描结果
 */
func (e *Engine) scanFile(filePath string, astMgr ast.ASTManager) *types.ScanResult {
	start := time.Now()
	result := &types.ScanResult{File: types.FileInfo{Path: filePath}}

	// 1. 获取文件信息和内容
	info, err := os.Stat(filePath)
	if err != nil {
		result.Error = fmt.Errorf("stat error: %w", err)
		logging.ErrorLogger.Printf("Error stating file %s: %v", filePath, err)
		result.Duration = time.Since(start)
		return result
	}
	result.File.Size = info.Size()
	result.File.ModTime = info.ModTime()

	// 基本大小检查
	const maxSize = 10 * 1024 * 1024 // 10MB 限制
	if info.Size() > maxSize {
		result.Error = fmt.Errorf("file exceeds size limit (%d > %d bytes)", info.Size(), maxSize)
		logging.WarnLogger.Printf("Skipping file %s: %v", filePath, result.Error)
		result.Duration = time.Since(start)
		return result
	}
	if info.Size() == 0 {
		logging.InfoLogger.Printf("Skipping empty file: %s", filePath)
		result.OverallRisk = types.RiskNone // Empty files are not risky
		result.Duration = time.Since(start)
		return result
	}

	// 读取文件内容
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		result.Error = fmt.Errorf("read error: %w", err)
		logging.ErrorLogger.Printf("Error reading file %s: %v", filePath, err)
		result.Duration = time.Since(start)
		return result
	}

	// 2. 获取 AST
	var goAST interface{}
	var astErr error
	if astMgr != nil {
		astStartTime := time.Now()
		goAST, astErr = astMgr.GetAST(content)
		astDuration := time.Since(astStartTime)
		if astErr != nil {
			logging.WarnLogger.Printf("AST generation failed for %s (Duration: %s): %v", filePath, astDuration, astErr)

		}
	} else {
		logging.InfoLogger.Printf("AST Manager not available, skipping AST generation for %s", filePath)
	}

	// 3. 提取特征
	featureSet, featErr := features.ExtractAllFeatures(result.File, content, goAST, astMgr)
	if featErr != nil {
		// Log the feature extraction error, but continue analysis if possible
		logging.WarnLogger.Printf("Feature extraction failed for %s: %v", filePath, featErr)
		// Allow analysis to continue with potentially incomplete features
	}
	// Ensure featureSet is not nil even if errors occurred, might be partially populated
	if featureSet == nil {
		featureSet = &features.FeatureSet{}
	}

	// 4. 运行所有启用的分析器
	var findings []*types.Finding
	analyzerStartTime := time.Now()

	// 获取启用的分析器名称并排序以确保确定性顺序
	enabledNames := make([]string, 0, len(e.analyzers))
	for name := range e.analyzers {
		enabledNames = append(enabledNames, name)
	}

	for _, name := range enabledNames {
		analyzer := e.analyzers[name]

		if e.canRunAnalyzer(analyzer, featureSet) {
			finding, analyzeErr := analyzer.Analyze(result.File, content, featureSet)
			if analyzeErr != nil {
				logging.WarnLogger.Printf("Analyzer '%s' failed on %s: %v", name, filePath, analyzeErr)
			}
			if finding != nil {
				findings = append(findings, finding)
			}
		} else {
			logging.InfoLogger.Printf("Skipping analyzer '%s' for %s: missing required features.", name, filePath)
		}
	}
	analyzerDuration := time.Since(analyzerStartTime)
	logging.InfoLogger.Printf("Analyzers finished for %s (Duration: %s)", filePath, analyzerDuration)

	// 5. 聚合得分
	result.Findings = findings
	result.OverallRisk = scoring.CalculateScore(result.Findings, featureSet)
	result.Duration = time.Since(start)

	logging.InfoLogger.Printf("Scan finished! Risk: %s, Findings: %d, Time: %s",
		result.OverallRisk.String(), len(result.Findings), result.Duration)
	return result
}

/**
 * @Description: 检查分析器所需的功能是否在FeatureSet中可用
 * @author: Mr wpl
 * @param analyzer Analyzer: 分析器
 * @param fs *features.FeatureSet: 特征集
 * @return bool: 是否可用
 */
func (e *Engine) canRunAnalyzer(analyzer Analyzer, fs *features.FeatureSet) bool {
	required := analyzer.RequiredFeatures()
	if len(required) == 0 {
		return true
	}
	if fs == nil {
		return false
	}

	for _, featureKey := range required {
		keyPresent := false
		switch strings.ToLower(featureKey) {
		case "statistical":
			keyPresent = fs.Statistical != nil
		case "ast_words":
			keyPresent = fs.ASTWords != nil // Check if slice is non-nil (means extraction was attempted)
		case "ast_op_sequence":
			keyPresent = fs.ASTOpSequence != nil // Check if slice is non-nil
		case "callable", "ast_callable": // Allow variation in key name
			// Callable is a boolean, always present in the struct if FeatureSet is not nil.
			// The check is more about whether AST analysis *could* run to set it.
			// If AST failed, fs.Callable might be false. Assume it's "present".
			keyPresent = true
		case "raw_ast":
			keyPresent = fs.RawAST != nil
		// Add checks for other feature keys as needed
		default:
			logging.WarnLogger.Printf("Analyzer '%s' requires check for unknown feature key '%s'", analyzer.Name(), featureKey)
			return false // Treat unknown requirement as missing
		}
		if !keyPresent {
			logging.InfoLogger.Printf("Analyzer '%s' missing required feature '%s'", analyzer.Name(), featureKey)
			return false // A required feature is missing
		}
	}
	return true
}

/**
 * @Description: 处理报告生成逻辑，支持html生成，默认终端命令生成
 * @author: Mr wpl
 * @param results []*types.ScanResult: 扫描结果
 * @param task *Task: 任务
 * @return error: 错误
 */
func (e *Engine) generateReport(results []*types.ScanResult, task *Task) error {
	// 1. Determine preferred reporter (console is default)
	var reporter reporting.Reporter = reporting.NewConsoleReporter() // Default to console
	outputFormat := strings.ToLower(e.config.Output.Format)          // Default from config
	outputPath := ""

	// Override format/path if -output flag was used
	if task.ReportPath != "" {
		outputPath = task.ReportPath
		outputExt := strings.ToLower(filepath.Ext(outputPath))
		logging.InfoLogger.Printf("Output path specified: %s (Extension: '%s')", outputPath, outputExt)
		switch outputExt {
		case ".html":
			reporter = reporting.NewHtmlReporter()
			outputFormat = "html"
		case ".json":
			outputFormat = "json"
			reporter = reporting.NewJsonReporter()
		case ".console", ".txt", "":
			outputFormat = "console"
			reporter = reporting.NewConsoleReporter()
			outputPath = ""
		default:
			logging.WarnLogger.Printf("Unsupported output file extension '%s' for path: %s. Using default '%s' reporter.", outputExt, outputPath, outputFormat)
			switch outputFormat {
			case "html":
				reporter = reporting.NewHtmlReporter()
				// Need a default path for HTML if only extension was bad?
				logging.WarnLogger.Printf("HTML output requires a path. Cannot save report.")
				return fmt.Errorf("cannot generate HTML report without a valid output path")
			default:
				reporter = reporting.NewConsoleReporter()
				outputPath = ""
			}
		}
	} else {
		// No -output flag, use config defaults
		switch outputFormat {
		case "html":
			reporter = reporting.NewHtmlReporter()
			// HTML needs a default path if not specified
			outputPath = "scan_report.html"
			logging.WarnLogger.Printf("HTML output format requires a path. Defaulting to '%s'", outputPath)
		case "json":
			reporter = reporting.NewJsonReporter()
			outputPath = ""
		default:
			reporter = reporting.NewConsoleReporter()
			outputPath = ""
		}
	}

	// 2. Generate the report using the selected reporter
	logging.InfoLogger.Printf("Generating '%s' report...", outputFormat)
	if err := reporter.Generate(results, outputPath); err != nil {
		// Log the specific reporter error
		logging.ErrorLogger.Printf("Failed to generate %s report: %v", outputFormat, err)
		if outputFormat != "console" {
			fmt.Fprintf(os.Stderr, "Error: Failed to generate report file '%s': %v\n", outputPath, err)
		}
		return fmt.Errorf("failed to generate %s report: %w", outputFormat, err)
	}

	if outputPath != "" {
		fmt.Println("Report generated: %s\n", outputPath) // Inform user about file creation
	}

	return nil
}

/**
 * @Description: 查找所有符合条件的php文件
 * @author: Mr wpl
 * @param paths []string: 需要扫描的文件或目录
 * @param exclusions []string: 需要排除的文件或目录
 * @return []string: 符合条件的php文件
 */
func findFiles(paths []string, exclusions []string) ([]string, error) {
	var files []string
	exclusionPatterns := make(map[string]bool)
	for _, ex := range exclusions {
		// Clean and normalize the exclusion path
		absEx, err := filepath.Abs(ex)
		if err == nil {
			exclusionPatterns[filepath.Clean(absEx)] = true
		} else {
			logging.WarnLogger.Printf("Could not get absolute path for exclusion '%s': %v", ex, err)
			exclusionPatterns[filepath.Clean(ex)] = true
		}
	}

	processedPaths := make(map[string]bool)

	for _, p := range paths {
		absP, err := filepath.Abs(p)
		if err != nil {
			logging.WarnLogger.Printf("Could not get absolute path for target '%s': %v. Skipping.", p, err)
			continue
		}
		cleanPath := filepath.Clean(absP)

		if processedPaths[cleanPath] {
			continue
		}

		// Check exclusion for the root path provided
		if exclusionPatterns[cleanPath] {
			logging.InfoLogger.Printf("Excluding path provided directly: %s", p)
			processedPaths[cleanPath] = true // Mark as processed even if excluded
			continue
		}

		info, err := os.Stat(cleanPath)
		if err != nil {
			logging.WarnLogger.Printf("Skipping path %s: %v", p, err)
			processedPaths[cleanPath] = true
			continue
		}

		if info.IsDir() {
			fmt.Println("Walking directory: %s", cleanPath)
			walkErr := filepath.Walk(cleanPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					logging.WarnLogger.Printf("Error accessing path %s during walk: %v", path, err)
					// Decide whether to skip file or directory based on error type
					// For now, just log and try to continue
					return nil
				}

				absWalkPath, walkAbsErr := filepath.Abs(path)
				if walkAbsErr != nil {
					return nil
				}
				cleanWalkPath := filepath.Clean(absWalkPath)

				// Check exclusion during walk
				if exclusionPatterns[cleanWalkPath] {
					if info.IsDir() {
						processedPaths[cleanWalkPath] = true
						return filepath.SkipDir
					}
					return nil
				}

				if !info.IsDir() {
					if processedPaths[cleanWalkPath] {
						return nil
					}
					// Filter by extension (e.g., only PHP)
					if strings.ToLower(filepath.Ext(path)) == ".php" {
						files = append(files, cleanWalkPath)
						processedPaths[cleanWalkPath] = true
					} else {
						fmt.Println("Skipping non-PHP file during walk: %s", path)
					}
				} else {
					processedPaths[cleanWalkPath] = true
				}
				return nil
			})
			if walkErr != nil {
				logging.ErrorLogger.Printf("Error walking directory %s: %v", cleanPath, walkErr)
			}
			processedPaths[cleanPath] = true
		} else {
			// Process single file
			if processedPaths[cleanPath] {
				continue
			}
			if strings.ToLower(filepath.Ext(cleanPath)) == ".php" {
				files = append(files, cleanPath)
			} else {
				logging.InfoLogger.Printf("Skipping non-PHP file specified directly: %s", p)
			}
			processedPaths[cleanPath] = true
		}
	}
	logging.InfoLogger.Printf("Found %d unique PHP files to scan.", len(files))
	return files, nil
}

// Task 定义需要扫描的内容
type Task struct {
	Paths        []string // 需要扫描的文件或目录
	Exclusions   []string // 需要排除的文件或目录
	ReportPath   string   // 保存报告的路径 (来自 -output)
	OutputFormat string   // Format is now determined by ReportPath or config
}
