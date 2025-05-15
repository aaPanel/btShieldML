package engine

import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/types"
)

// Analyzer defines the interface for all detection methods.
type Analyzer interface {
	Name() string                                                                                             // Returns the unique name of the analyzer
	Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) // Pass content directly
	RequiredFeatures() []string                                                                               // List feature keys this analyzer needs (e.g., ["statistical", "ast_op_sequence"])
}

// Reporter defines the interface for generating output reports.
type Reporter interface {
	Generate(results []*types.ScanResult, outputPath string) error
}

// ASTManager defines the interface for getting AST data.
type ASTManager interface {
	GetAST(source []byte) (astData []byte, err error) // Returns raw AST data (e.g., JSON)
	Cleanup() error                                   // Cleans up any resources (e.g., PHP process)
}
