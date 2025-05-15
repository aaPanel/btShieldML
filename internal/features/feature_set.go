package features

/*
 * @Author: wpl
 * @Date: 2025-04-15 10:24:13
 * @LastEditors: Please set LastEditors
 * @LastEditTime: 2025-05-07 15:28:09
 * @Description: 待提取特征列表信息
 */

// FeatureSet holds all extracted features for a file.
type FeatureSet struct {
	Statistical   *StatisticalFeatures // Pointer to allow nil if not calculated
	ASTWords      []string             // Extracted words from AST
	ASTOpSequence [][]int              // Extracted operation sequences from AST
	Callable      bool                 // Flag indicating if critical callable functions were found in AST
	// Add more feature categories as needed
	RawAST interface{} // Store the parsed Go AST if needed by multiple analyzers
}

// StatisticalFeatures holds features calculated by statistical analyzer.
// NOTE: This calculation happens *before* AST analysis in the feature extractor,
// based solely on file content.
type StatisticalFeatures struct {
	LM  float64 // Line Max Length
	LVC float64 // Line Variation Coefficient
	WM  float64 // Word Max Length
	WVC float64 // Word Variation Coefficient
	SR  float64 // Symbol Ratio
	TR  float64 // Tag Ratio
	SPL float64 // Statements Per Line
	IE  float64 // Information Entropy
}
