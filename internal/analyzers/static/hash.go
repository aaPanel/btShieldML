/*
 * @Date: 2025-04-15 10:29:24
 * @Editors: Mr wpl
 * @Description:
 */
package static

import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type HashAnalyzer struct {
	analyzerName string // Renamed field to avoid conflict
	badHashes    map[string]bool
}

/**
 * @Description: 创建HashAnalyzer实例
 * @author: Mr wpl
 * @param dataPath 数据路径
 * @return *HashAnalyzer 哈希分析器实例
 * @return error 错误信息
 */
func NewHashAnalyzer(dataPath string) (*HashAnalyzer, error) {
	hashes := make(map[string]bool)
	hashFilePath := filepath.Join(dataPath, "SampleHash.txt")
	file, err := os.Open(hashFilePath)
	if err != nil {
		logging.WarnLogger.Printf("Hash signature file not found at %s: %v. Hash analyzer will be inactive.", hashFilePath, err)
		return &HashAnalyzer{analyzerName: "hash", badHashes: hashes}, nil // Use renamed field here
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		hash := strings.TrimSpace(scanner.Text())
		if len(hash) == 64 {
			hashes[strings.ToLower(hash)] = true
		} else if hash != "" && !strings.HasPrefix(hash, "#") {
			logging.WarnLogger.Printf("Invalid hash format on line %d in %s: %s", lineNum, hashFilePath, hash)
		}
	}
	if err := scanner.Err(); err != nil {
		logging.ErrorLogger.Printf("Error reading hash file %s: %v", hashFilePath, err)
	}

	logging.InfoLogger.Printf("Loaded %d bad hashes from %s", len(hashes), hashFilePath)
	return &HashAnalyzer{analyzerName: "hash", badHashes: hashes}, nil // Use renamed field here
}
	
/**
 * @Description: 返回分析器名称
 * @author: Mr wpl
 * @return string 分析器名称
 */
func (a *HashAnalyzer) Name() string {
	return a.analyzerName // Return the value of the renamed field
}

/**
 * @Description: 返回分析器所需的特征
 * @author: Mr wpl
 * @return []string 分析器所需的特征
 */
func (a *HashAnalyzer) RequiredFeatures() []string {
	return []string{}
}

/**
 * @Description: 分析文件
 * @author: Mr wpl
 * @param fileInfo 文件信息
 * @param content 文件内容
 * @param featureSet 特征集
 */
func (a *HashAnalyzer) Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) {
	if len(a.badHashes) == 0 {
		return nil, nil
	}

	hasher := sha256.New()
	if _, err := hasher.Write(content); err != nil {
		return nil, fmt.Errorf("failed to calculate hash: %w", err)
	}
	hashString := hex.EncodeToString(hasher.Sum(nil))

	if a.badHashes[strings.ToLower(hashString)] {
		logging.InfoLogger.Printf("Hash match found for %s", fileInfo.Path)
		return &types.Finding{
			AnalyzerName: a.analyzerName, // Use renamed field here
			Description:  fmt.Sprintf("Matched known bad file hash: %s", hashString),
			Risk:         types.RiskCritical,
			Confidence:   1.0,
		}, nil
	}

	return nil, nil
}
