/*
 * @Date: 2025-04-15 10:26:28
 * @Editors: Mr wpl
 * @Description: 特征提取器
 */
package features

import (
	"bt-shieldml/internal/ast"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"fmt"
)

// ExtractAllFeatures 协调各种特征的提取。
// 注意：现在接收 AST 管理器接口或解析后的 AST 本身。
// 为了简化 engine.go 的调用，我们直接传入解析后的 AST (goAST interface{})。
func ExtractAllFeatures(fileInfo types.FileInfo, content []byte, goAST interface{}, astMgr ast.ASTManager) (*FeatureSet, error) {
	fs := &FeatureSet{
		RawAST: goAST, // Store raw AST
	}
	var errs []error // Collect errors

	// 1. Statistical Features (calculated directly using functions in this package)
	if len(content) > 0 {
		// logging.InfoLogger.Printf("Extracting statistical features for %s", fileInfo.Path)
		// Call the function directly as it's now in the 'features' package
		calculatedStats := CalculateStatisticalFeatures(content)
		fs.Statistical = &calculatedStats
	} else {
		// Handle empty content - statistical features will be nil
		logging.InfoLogger.Printf("Skipping statistical feature calculation for empty file: %s", fileInfo.Path)
	}

	// 2. AST-based Features (only if AST is available and manager is provided)
	if goAST != nil && astMgr != nil {

		// Extract Words and Callable status
		words, callable, wordsErr := astMgr.GetWordsAndCallable(goAST)
		if wordsErr != nil {
			logging.WarnLogger.Printf("Could not extract words/callable from AST for %s: %v", fileInfo.Path, wordsErr)
			errs = append(errs, fmt.Errorf("ast words/callable extraction failed: %w", wordsErr))
			// fs.ASTWords remains nil
			// fs.Callable remains false (default)
		} else {
			fs.ASTWords = words
			fs.Callable = callable // Set the extracted callable status
		}

		// Extract Operation Sequence
		opSeq, opSeqErr := astMgr.GetOpSerial(goAST)
		if opSeqErr != nil {
			logging.WarnLogger.Printf("Could not extract op sequence from AST for %s: %v", fileInfo.Path, opSeqErr)
			errs = append(errs, fmt.Errorf("ast op sequence extraction failed: %w", opSeqErr))
		} else {
			fs.ASTOpSequence = opSeq
			// logging.InfoLogger.Printf("AST OpSequence (%d sequences) extracted for %s", len(fs.ASTOpSequence), fileInfo.Path)
		}

	} else if goAST == nil {
		logging.InfoLogger.Printf("Skipping AST-based feature extraction for %s (no AST available)", fileInfo.Path)
	} else {
		logging.WarnLogger.Printf("Skipping AST-based feature extraction for %s (AST Manager not provided to extractor)", fileInfo.Path)
		errs = append(errs, fmt.Errorf("ast manager was nil during feature extraction"))

	}

	// Combine errors if needed
	var combinedErr error
	if len(errs) > 0 {
		errMsg := ""
		for i, e := range errs {
			errMsg += e.Error()
			if i < len(errs)-1 {
				errMsg += "; "
			}
		}
		combinedErr = fmt.Errorf("feature extraction encountered errors: %s", errMsg)
	}

	return fs, combinedErr
}
