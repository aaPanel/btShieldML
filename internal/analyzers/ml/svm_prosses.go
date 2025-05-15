package ml
/*
 * @Date: 2025-05-14 17:10:41
 * @Editors: Mr wpl
 * @Description: 融合8大统计特征+朴素贝叶斯模型预测
 */
import (
	"bt-shieldml/internal/features"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"path/filepath"

	"bt-shieldml/pkg/embedded"

	libSvm "github.com/CyrusF/libsvm-go"
)

// FeatureInfo 存储特征信息
type FeatureInfo struct {
	Name   string  `json:"name"`
	Weight float32 `json:"weight"`
}

// CalibrationInfo 存储SVM模型的校准信息
type CalibrationInfo struct {
	FeatureNames      []string                    `json:"feature_names"`
	NumFeatures       int                         `json:"num_features"`
	FeatureStats      FeatureStats                `json:"feature_stats"`
	SigmoidParams     SigmoidParams               `json:"sigmoid_params"`
	OptimalThreshold  float64                     `json:"optimal_threshold"`
	ClassMapping      map[string]string           `json:"class_mapping"`
	ValidationSamples map[string]ValidationSample `json:"validation_samples"`
}

// FeatureStats 存储特征的统计信息
type FeatureStats struct {
	Mins  []float64 `json:"mins"`
	Maxs  []float64 `json:"maxs"`
	Means []float64 `json:"means"`
	Stds  []float64 `json:"stds"`
}

// SigmoidParams Sigmoid函数参数
type SigmoidParams struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

// ValidationSample 验证样本信息
type ValidationSample struct {
	Features      []float64 `json:"features"`
	RawDecision   float64   `json:"raw_decision"`
	SigmoidScore  float64   `json:"sigmoid_score"`
	ExpectedClass string    `json:"expected_class"`
}

// SvmProssesAnalyzer 实现SVM处理分析器
type SvmProssesAnalyzer struct {
	modelPath           string
	featureInfos        []FeatureInfo
	model               *libSvm.Model
	bayesModel          *BayesWordsAnalyzer
	isInitialized       bool
	featureNames        []string
	calibration         CalibrationInfo
	validationPerformed bool
	validationPassed    bool
}

// NewSvmProssesAnalyzer
/**
 * @Description: 初始化SVM处理分析器
 * @param modelPath 模型文件路径
 * @return *SvmProssesAnalyzer 分析器实例
 * @return error 错误信息
 */
func NewSvmProssesAnalyzer(modelPath string) (*SvmProssesAnalyzer, error) {
	analyzer := &SvmProssesAnalyzer{
		modelPath:     modelPath,
		isInitialized: false,
		featureNames:  []string{"LM", "LVC", "WM", "WVC", "SR", "TR", "SPL", "IE", "BAYES"},
	}

	// 1. 加载校准信息（info文件）
	infoData, err := embedded.GetFileContent("data/models/ProcessSVM.model.info")
	if err != nil {
		logging.WarnLogger.Printf("未找到嵌入的SVM校准信息，尝试从磁盘加载: %v", err)

		// 尝试从磁盘加载
		infoFilePath := filepath.Join(modelPath, "ProcessSVM.model.info")
		infoData, err = ioutil.ReadFile(infoFilePath)
		if err != nil {
			logging.WarnLogger.Printf("加载SVM校准信息失败: %v，分析器将处于非活动状态。", err)
			return analyzer, nil
		}
	}

	// 解析校准信息
	err = json.Unmarshal(infoData, &analyzer.calibration)
	if err != nil {
		logging.ErrorLogger.Printf("解析SVM校准信息失败: %v", err)
		return analyzer, nil
	}

	// 2. 加载特征信息（使用同一个info文件）
	err = analyzer.loadFeatureInfo(infoData)
	if err != nil {
		logging.WarnLogger.Printf("解析特征信息失败: %v", err)
	}

	// 3. 加载朴素贝叶斯模型
	bayesModel, err := NewBayesWordsAnalyzer(modelPath)
	if err != nil {
		logging.WarnLogger.Printf("加载朴素贝叶斯模型失败: %v，可能无法获取所有特征。", err)
	}
	analyzer.bayesModel = bayesModel

	// 4. 加载SVM模型
	modelData, err := embedded.GetFileContent("data/models/ProcessSVM.model.model")
	if err != nil {
		logging.WarnLogger.Printf("未找到嵌入的SVM模型，尝试从磁盘加载: %v", err)

		// 尝试从磁盘加载
		modelFilePath := filepath.Join(modelPath, "ProcessSVM.model.model")
		modelData, err = ioutil.ReadFile(modelFilePath)
		if err != nil {
			logging.WarnLogger.Printf("读取SVM模型文件失败: %v", err)
			return analyzer, nil
		}
	}

	// 使用libSvm库加载模型
	modelReader := bytes.NewReader(modelData)
	analyzer.model = libSvm.NewModelFromFileStream(modelReader)

	// 验证模型是否正确加载
	if analyzer.model == nil {
		logging.ErrorLogger.Printf("SVM模型加载失败，返回值为nil")
		return analyzer, nil
	}

	// 执行模型验证
	analyzer.validateModel()

	analyzer.isInitialized = true

	return analyzer, nil
}


/**
 * @Description: 从JSON文件加载特征信息
 * @param data 特征信息数据
 * @return error 错误信息
 */
func (s *SvmProssesAnalyzer) loadFeatureInfo(data []byte) error {
	// 解析特征信息
	var featureInfo struct {
		FeatureNames []string `json:"feature_names"`
		NumFeatures  int      `json:"num_features"`
	}

	err := json.Unmarshal(data, &featureInfo)
	if err != nil {
		return err
	}

	// 如果特征名称存在，则使用它们
	if len(featureInfo.FeatureNames) > 0 {
		s.featureNames = featureInfo.FeatureNames
	}

	// 构建特征信息列表
	s.featureInfos = make([]FeatureInfo, len(s.featureNames))
	for i, name := range s.featureNames {
		s.featureInfos[i] = FeatureInfo{
			Name:   name,
			Weight: 1.0, // 默认权重
		}
	}

	return nil
}


/**
 * @Description: 从JSON文件加载校准信息
 * @param data 校准信息数据
 * @return error 错误信息
 */
func (s *SvmProssesAnalyzer) loadCalibrationInfo(data []byte) error {
	// 解析校准信息
	err := json.Unmarshal(data, &s.calibration)
	if err != nil {
		logging.ErrorLogger.Printf("解析校准信息JSON数据失败: %v", err)
		return err
	}

	// 检查是否完整
	if len(s.calibration.FeatureNames) == 0 || s.calibration.NumFeatures == 0 {
		logging.WarnLogger.Printf("校准信息不完整: 特征名称或特征数量缺失")
		return fmt.Errorf("校准信息不完整：特征名称或数量缺失")
	}

	// 检查Sigmoid参数是否有效
	if s.calibration.SigmoidParams.A == 0 {
		logging.WarnLogger.Printf("Sigmoid参数A为0，设置为默认值1.0")
		s.calibration.SigmoidParams.A = 1.0
	}

	// 检查阈值是否有效
	if s.calibration.OptimalThreshold <= 0 || s.calibration.OptimalThreshold >= 1 {
		logging.WarnLogger.Printf("最优阈值无效(%.4f)，设置为默认值0.5", s.calibration.OptimalThreshold)
		s.calibration.OptimalThreshold = 0.5
	}

	// 检查特征统计信息是否完整
	if s.calibration.FeatureStats.Means == nil || len(s.calibration.FeatureStats.Means) < s.calibration.NumFeatures {
		logging.WarnLogger.Printf("特征统计信息不完整: 均值缺失")
		return fmt.Errorf("特征统计信息不完整：均值缺失")
	}

	if s.calibration.FeatureStats.Stds == nil || len(s.calibration.FeatureStats.Stds) < s.calibration.NumFeatures {
		logging.WarnLogger.Printf("特征统计信息不完整: 标准差缺失")
		return fmt.Errorf("特征统计信息不完整：标准差缺失")
	}

	// 使用校准信息中的特征名称，如果存在
	if len(s.calibration.FeatureNames) >= len(s.featureNames) {
		s.featureNames = s.calibration.FeatureNames
	}

	logging.InfoLogger.Printf("成功加载SVM校准信息，最佳阈值: %.4f, Sigmoid参数: a=%.4f, b=%.4f",
		s.calibration.OptimalThreshold, s.calibration.SigmoidParams.A, s.calibration.SigmoidParams.B)
	return nil
}


/**
 * @Description: 验证模型一致性
 * @author: Mr wpl
 * @return void
 */
func (s *SvmProssesAnalyzer) validateModel() {
	// 如果模型未初始化，跳过验证
	if !s.isInitialized || s.model == nil {
		// logging.WarnLogger.Printf("模型未初始化，跳过验证")
		return
	}

	// 标记已执行验证
	s.validationPerformed = true

	// 检查验证样本数量
	if len(s.calibration.ValidationSamples) == 0 {
		logging.WarnLogger.Printf("没有验证样本，跳过验证")
		return
	}

	logging.InfoLogger.Printf("开始验证SVM模型一致性...")

	// 默认通过验证
	s.validationPassed = true
	correctCount := 0
	totalCount := 0

	for sampleName, sample := range s.calibration.ValidationSamples {
		// 准备特征
		features := make(map[int]float64)
		for i, val := range sample.Features {
			features[i+1] = val // 特征索引从1开始
		}

		// 执行预测
		_, rawValues := s.model.PredictValues(features)

		// 如果没有决策值，跳过
		if len(rawValues) == 0 {
			logging.WarnLogger.Printf("样本 %s 预测失败：没有决策值", sampleName)
			continue
		}

		rawScore := rawValues[0]

		// 应用sigmoid转换
		sigmoidScore := s.applySigmoid(rawScore)

		// 预测类别
		predictedClass := "normal"
		if sigmoidScore >= s.calibration.OptimalThreshold {
			predictedClass = "webshell"
		}

		// 检查与预期是否一致
		isCorrect := predictedClass == sample.ExpectedClass

		// 记录验证结果
		totalCount++
		if isCorrect {
			correctCount++
		} else {
			logging.WarnLogger.Printf("验证样本 %s 预测错误: 期望=%s, 实际=%s, 分数=%.4f (原始决策值=%.4f)",
				sampleName, sample.ExpectedClass, predictedClass, sigmoidScore, rawScore)

			// 检查方向性是否正确
			expectedSignDirection := 1.0
			if sample.ExpectedClass == "normal" {
				expectedSignDirection = -1.0
			}

			actualSignDirection := 1.0
			if rawScore < 0 {
				actualSignDirection = -1.0
			}

			// 如果方向不一致，可能需要反转决策
			if expectedSignDirection != actualSignDirection {
				logging.ErrorLogger.Printf("验证失败: 模型决策方向与预期不符，可能需要反转决策")
				s.validationPassed = false
			}
		}
	}

	// 检查准确率
	if totalCount > 0 {
		accuracy := float64(correctCount) / float64(totalCount)
		logging.InfoLogger.Printf("模型验证完成: 准确率=%.2f (%d/%d)", accuracy, correctCount, totalCount)

		// 如果准确率过低，标记验证不通过
		if accuracy < 0.5 {
			logging.WarnLogger.Printf("验证准确率过低(%.2f)，模型可能存在问题", accuracy)
			s.validationPassed = false
		}
	}
}

/**
 * @Description: 返回分析器名称
 * @author: Mr wpl
 * @return string 分析器名称
 */
func (s *SvmProssesAnalyzer) Name() string {
	return "svm_prosses"
}

/**
 * @Description: 返回此分析器所需的特征
 * @author: Mr wpl
 * @return []string 分析器所需的特征
 */
func (s *SvmProssesAnalyzer) RequiredFeatures() []string {
	return []string{"statistical", "ast_words"}
}

/**
 * @Description: 实现Analyzer接口的Analyze方法
 * @author: Mr wpl
 * @param fileInfo 文件信息
 * @param content 文件内容
 * @param featureSet 特征集
 * @return *types.Finding 发现
 * @return error 错误信息
 */
func (s *SvmProssesAnalyzer) Analyze(fileInfo types.FileInfo, content []byte, featureSet *features.FeatureSet) (*types.Finding, error) {
	if !s.isInitialized || s.model == nil {
		logging.InfoLogger.Printf("SVM Prosses分析器未初始化或模型为空，跳过分析: %s", fileInfo.Path)
		return nil, nil
	}

	// 检查必需的特征是否存在
	if featureSet == nil || featureSet.Statistical == nil {
		logging.WarnLogger.Printf("缺少必要的统计特征，无法进行SVM分析: %s", fileInfo.Path)
		return nil, fmt.Errorf("SvmProssesAnalyzer: 缺少必需的statistical特征集")
	}

	// 1. 提取特征
	features, err := s.extractFeatures(fileInfo.Path, content, featureSet)
	if err != nil {
		logging.WarnLogger.Printf("特征提取失败: %v", err)
		return nil, err
	}

	// 2. 使用SVM模型预测
	score, rawScore, err := s.predict(features)
	if err != nil {
		logging.WarnLogger.Printf("模型预测失败: %v", err)
		return nil, err
	}

	// 3. 根据校准的阈值决定是否返回发现
	threshold := 0.95

	if score >= threshold {
		confidence := score
		description := fmt.Sprintf("融合特征分析检测到可疑代码 (8大统计特征+朴素贝叶斯评分: %.4f, 原始决策值: %.4f)", score, rawScore)

		return &types.Finding{
			AnalyzerName: s.Name(),
			Description:  description,
			Risk:         types.RiskHigh,
			Confidence:   confidence,
		}, nil
	}

	return nil, nil
}

/**
 * @Description: 提取文件的特征
 * @param filepath 文件路径
 * @param content 文件内容
 * @param featureSet 特征集
 * @return map[int]float64 特征
 * @return error 错误信息
 */
func (s *SvmProssesAnalyzer) extractFeatures(filepath string, content []byte, featureSet *features.FeatureSet) (map[int]float64, error) {
	features := make(map[int]float64)

	// 1. 从featureSet获取8个统计特征
	statFeatures := featureSet.Statistical

	// 将8个统计特征添加到特征映射
	features[1] = s.normalizeFeature(float64(statFeatures.LM), 0, 1)  // 行长度最大值
	features[2] = s.normalizeFeature(float64(statFeatures.LVC), 1, 2) // 行变异系数
	features[3] = s.normalizeFeature(float64(statFeatures.WM), 2, 3)  // 词长度最大值
	features[4] = s.normalizeFeature(float64(statFeatures.WVC), 3, 4) // 词变异系数
	features[5] = s.normalizeFeature(float64(statFeatures.SR), 4, 5)  // 符号比率
	features[6] = s.normalizeFeature(float64(statFeatures.TR), 5, 6)  // 标签比率
	features[7] = s.normalizeFeature(float64(statFeatures.SPL), 6, 7) // 每行语句数
	features[8] = s.normalizeFeature(float64(statFeatures.IE), 7, 8)  // 信息熵

	// 2. 获取朴素贝叶斯分数作为特征
	var bayesScore float64 = 0.5
	if s.bayesModel != nil && featureSet.ASTWords != nil && len(featureSet.ASTWords) > 0 {
		// 使用已提取的AST词汇直接调用分析
		finding, err := s.bayesModel.Analyze(types.FileInfo{Path: filepath}, content, featureSet)
		if err == nil && finding != nil {
			// 如果分析成功且有发现，使用置信度作为分数
			bayesScore = finding.Confidence
		} else {
			logging.InfoLogger.Printf("朴素贝叶斯评分获取失败，使用默认值0.5: %v", err)
		}
	} else {
		logging.InfoLogger.Printf("朴素贝叶斯模型不可用或AST词汇为空，使用默认评分0.5")
	}

	features[9] = s.normalizeFeature(bayesScore, 8, 9)

	return features, nil
}

/**
 * @Description: 对特征值进行标准化和异常值处理
 * @author: Mr wpl
 * @param value 特征值
 * @param meanIdx 均值索引
 * @param stdIdx 标准差索引
 * @return float64 标准化后的特征值
 */
func (s *SvmProssesAnalyzer) normalizeFeature(value float64, meanIdx int, stdIdx int) float64 {
	// 检查是否有可用的统计信息
	if len(s.calibration.FeatureStats.Means) <= meanIdx || len(s.calibration.FeatureStats.Stds) <= stdIdx {
		return value // 如果无法标准化，返回原始值
	}

	// 获取均值和标准差
	mean := s.calibration.FeatureStats.Means[meanIdx]
	std := s.calibration.FeatureStats.Stds[stdIdx]

	// 异常值处理 - 截断极端值
	if len(s.calibration.FeatureStats.Mins) > meanIdx && len(s.calibration.FeatureStats.Maxs) > meanIdx {
		min := s.calibration.FeatureStats.Mins[meanIdx]
		max := s.calibration.FeatureStats.Maxs[meanIdx]

		// 允许一定程度的超出范围，使用训练集范围的1.5倍
		extendedMin := min - 0.5*(max-min)
		extendedMax := max + 0.5*(max-min)

		if value < extendedMin {
			value = extendedMin
		} else if value > extendedMax {
			value = extendedMax
		}
	}

	// 标准化
	if std > 0 {
		return (value - mean) / std
	}
	return 0.0
}

/**
 * @Description: 使用SVM模型进行预测
 * @author: Mr wpl
 * @param features 特征
 * @return float64 分数
 * @return float64 原始分数
 * @return error 错误信息
 */
func (s *SvmProssesAnalyzer) predict(features map[int]float64) (float64, float64, error) {
	// 检查模型是否已初始化
	if s.model == nil {
		logging.WarnLogger.Printf("SVM模型未初始化，返回默认分数0.5")
		return 0.5, 0.0, nil
	}

	// 检查验证是否通过
	if s.validationPerformed && !s.validationPassed {
		logging.WarnLogger.Printf("SVM模型验证不通过，可能需要反转决策")
	}

	// 直接使用libSVM的PredictValues方法进行预测
	_, result := s.model.PredictValues(features)

	// 结果为空的安全处理
	if len(result) == 0 {
		return 0.5, 0.0, nil
	}

	// 获取原始分数（决策值）
	rawScore := result[0]

	// 如果验证未通过且检测到决策方向问题，反转决策值
	if s.validationPerformed && !s.validationPassed {
		rawScore = -rawScore
	}

	// 使用校准的sigmoid参数将决策值转换为概率
	normalizedScore := s.applySigmoid(rawScore)

	return normalizedScore, rawScore, nil
}

/**
 * @Description: 应用sigmoid函数将原始决策值转换为[0,1]概率
 * @author: Mr wpl
 * @param rawScore 原始分数
 * @return float64 概率
 */
func (s *SvmProssesAnalyzer) applySigmoid(rawScore float64) float64 {
	// 获取校准的sigmoid参数
	a := s.calibration.SigmoidParams.A
	b := s.calibration.SigmoidParams.B

	// 应用sigmoid函数 1/(1+exp(-a*(x-b)))
	return 1.0 / (1.0 + math.Exp(-a*(rawScore-b)))
}

/**
 * @Description: 释放资源
 * @author: Mr wpl
 * @return error 错误信息
 */
func (s *SvmProssesAnalyzer) Close() error {
	s.model = nil
	s.bayesModel = nil
	return nil
}
