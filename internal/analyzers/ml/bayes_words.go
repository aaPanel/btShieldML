/*
 * @Date: 2025-04-21 17:11:17
 * @Editors: Mr wpl
 * @Description: 朴素贝叶斯模型预测
 */
package ml

import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"

	"bt-shieldml/pkg/embedded"

	"github.com/CyrusF/go-bayesian"
)

// --- 定义与 Python 保存的 JSON 格式匹配的 Go 结构体 ---

// 用于存储每个类别（normal/webshell）的数据
type classData struct {
	DocCount       int            `json:"docCount"`       // 该类别的文档数量
	WordCount      map[string]int `json:"wordCount"`      // 该类别下每个词出现的次数
	TotalWordCount int            `json:"totalWordCount"` // 该类别下所有词的总数
}

// 用于存储整个模型文件的 JSON 数据
type goBayesianModelData struct {
	Normal             classData `json:"normal"`             // "normal" 类别的数据
	Webshell           classData `json:"webshell"`           // "webshell" 类别的数据
	TotalDocumentCount int       `json:"totalDocumentCount"` // 所有类别的总文档数
}

// --- 结构体定义结束 ---

type BayesWordsAnalyzer struct {
	analyzerName  string
	classifier    bayesian.Classifier
	isInitialized bool
}

func NewBayesWordsAnalyzer(modelPath string) (*BayesWordsAnalyzer, error) {
	analyzer := &BayesWordsAnalyzer{
		analyzerName:  "bayes_words",
		isInitialized: false,
	}

	// 优先从嵌入文件加载
	jsonData, err := embedded.GetFileContent("data/models/Words.model")
	if err != nil {
		logging.WarnLogger.Printf("未找到嵌入的Bayes模型，尝试从磁盘加载: %v", err)

		// 尝试从磁盘加载
		wordModelPath := filepath.Join(modelPath, "Words.model")
		modelFile, err := os.Open(wordModelPath)
		if err != nil {
			logging.WarnLogger.Printf("无法打开 Bayes Words 模型文件 %s: %v。分析器将处于非活动状态。", wordModelPath, err)
			return analyzer, nil
		}
		defer modelFile.Close()

		jsonData, err = ioutil.ReadAll(modelFile)
		if err != nil {
			logging.ErrorLogger.Printf("无法读取 Bayes Words 模型文件 %s: %v", wordModelPath, err)
			return nil, fmt.Errorf("读取 bayes 模型文件失败: %w", err)
		}
	}

	// 解析JSON数据
	var modelData goBayesianModelData
	err = json.Unmarshal(jsonData, &modelData)
	if err != nil {
		logging.ErrorLogger.Printf("无法解析Bayes Words模型JSON: %v", err)
		logging.ErrorLogger.Printf("JSON前100字节: %s", string(jsonData[:min(100, len(jsonData))]))
		return nil, fmt.Errorf("解析bayes模型JSON失败: %w", err)
	}

	// --- 第 3 步: 手动构建 bayesian.Classifier 对象 ---
	// 根据 JSON 数据定义分类器的类别
	normalClass := bayesian.Class("normal")
	webshellClass := bayesian.Class("webshell")

	// 初始化分类器内部需要的 map
	learningResults := make(map[string]map[bayesian.Class]int) // 存储 <词, <类别, 次数>>
	nDocByClass := make(map[bayesian.Class]int)                // 存储 <类别, 文档数>
	nFreqByClass := make(map[bayesian.Class]int)               // 存储 <类别, 总词频>
	priorProbabilities := make(map[bayesian.Class]float64)     // 存储 <类别, 先验概率>

	// --- 填充 "normal" 类的数据 ---
	nDocByClass[normalClass] = modelData.Normal.DocCount
	nFreqByClass[normalClass] = modelData.Normal.TotalWordCount
	for word, count := range modelData.Normal.WordCount {
		// 如果是第一次遇到这个词，先初始化内部 map
		if _, ok := learningResults[word]; !ok {
			learningResults[word] = make(map[bayesian.Class]int)
		}
		learningResults[word][normalClass] = count // 记录该词在 normal 类中的次数
	}

	// --- 填充 "webshell" 类的数据 ---
	nDocByClass[webshellClass] = modelData.Webshell.DocCount
	nFreqByClass[webshellClass] = modelData.Webshell.TotalWordCount
	for word, count := range modelData.Webshell.WordCount {
		if _, ok := learningResults[word]; !ok {
			learningResults[word] = make(map[bayesian.Class]int)
		}
		learningResults[word][webshellClass] = count // 记录该词在 webshell 类中的次数
	}

	// --- 计算先验概率 (对数形式，因为 Classify 内部使用对数) ---
	totalDocs := float64(modelData.TotalDocumentCount)
	normalDocs := float64(nDocByClass[normalClass])
	webshellDocs := float64(nDocByClass[webshellClass])

	if totalDocs > 0 {
		// 使用 Log 以匹配 bayesian.Classifier 内部计算
		priorProbabilities[normalClass] = math.Log(normalDocs / totalDocs)
		priorProbabilities[webshellClass] = math.Log(webshellDocs / totalDocs)
	} else {
		// 处理总文档数为 0 的情况
		priorProbabilities[normalClass] = math.Log(0.5) // 对数先验概率
		priorProbabilities[webshellClass] = math.Log(0.5)

	}

	// --- 创建最终的 classifier 对象 ---
	analyzer.classifier = bayesian.Classifier{
		Model: bayesian.MultinomialTf, 
		// 注意：go-bayesian 库的 PriorProbabilities 字段存储的是对数先验概率
		PriorProbabilities: priorProbabilities,           // 存储计算出的对数先验概率
		LearningResults:    learningResults,              // 设置学习结果 (词频统计)
		NDocumentByClass:   nDocByClass,                  // 设置各类别的文档数
		NFrequencyByClass:  nFreqByClass,                 // 设置各类别的总词频
		NAllDocument:       modelData.TotalDocumentCount, // 设置总文档数
	}

	analyzer.isInitialized = true
	return analyzer, nil
}

func (a *BayesWordsAnalyzer) Name() string {
	return a.analyzerName
}

func (a *BayesWordsAnalyzer) RequiredFeatures() []string {
	// Needs the words extracted from the AST
	return []string{"ast_words"}
}

func (a *BayesWordsAnalyzer) Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) {
	// 1. 检查分析器是否已初始化
	if !a.isInitialized {
		// 分析器未成功加载模型，不执行分析
		fmt.Printf("BayesWordsAnalyzer: 模型未成功加载，跳过分析文件 %s\n", fileInfo.Path)
		return nil, nil
	}

	// 2. 检查必需的特征是否存在
	if featureSet == nil || featureSet.ASTWords == nil {
		// 如果在 featureSet 为 nil 时也应分析，则调整此逻辑
		return nil, fmt.Errorf("BayesWordsAnalyzer: 缺少必需的 ast_words 特征集")
	}

	words := featureSet.ASTWords
	// 3. 如果没有提取到单词，则无法进行分析
	if len(words) == 0 {
		fmt.Printf("BayesWordsAnalyzer: 文件 %s 没有提取到任何单词", fileInfo.Path)
		return nil, nil // 没有单词，无法分类
	}

	// 4. 使用分类器进行分类，获取原始对数概率
	allLogScores, predictedClass, _ := a.classifier.Classify(words...)

	// // 5. 只关心预测为 "webshell" 的情况
	// if predictedClass != "webshell" {
	// 	fmt.Printf("BayesWordsAnalyzer: 文件 %s 未被预测为 webshell\n", fileInfo.Path)
	// 	return nil, nil
	// }

	// 6. 将对数概率转换为归一化概率以计算置信度 ---
	logProbWebshell, okWebshell := allLogScores["webshell"]
	logProbNormal, okNormal := allLogScores["normal"]

	// 健壮性检查：确保两个类别的分数都存在
	if !okWebshell || !okNormal {
		fmt.Printf("BayesWordsAnalyzer: 文件 %s 的概率计算失败\n", fileInfo.Path)
		return nil, nil
	}

	// 为了数值稳定性，在指数化前减去最大对数概率
	maxLogProb := math.Max(logProbWebshell, logProbNormal)
	probWebshell := math.Exp(logProbWebshell - maxLogProb)
	probNormal := math.Exp(logProbNormal - maxLogProb)

	// 计算归一化概率（置信度）
	totalProb := probWebshell + probNormal

	var confidence float64
	if totalProb > 1e-9 { // 避免除以接近零的数
		confidence = probWebshell / totalProb
	} else {
		fmt.Printf("BayesWordsAnalyzer: 文件 %s 的概率计算导致总概率为零\n", fileInfo.Path)
		return nil, nil
	}

	// --- 7. 构建并返回发现 ---
	return &types.Finding{
		AnalyzerName: a.Name(),
		Description:  fmt.Sprintf("Bayes Words 模型预测为 (类别: %s, 置信度: %.4f)", predictedClass, confidence),
		Risk:         types.RiskMedium,
		Confidence:   confidence,
	}, nil
}

// min 函数 (用于日志截断)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
