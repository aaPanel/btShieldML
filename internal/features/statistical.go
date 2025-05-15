package features

/*
 * @Author: wpl
 * @Date: 2025-04-15 10:24:13
 * @Description: 统计特征提取，基于CloudWalker实现
 */
import (
	"math"
	"regexp"
	"strings"

	"github.com/grd/stat"
)

/**
 * @Description: 计算给定内容的所有 8 个统计特征
 * @author: Mr wpl
 * @param content []byte: 内容
 * @return StatisticalFeatures: 统计特征
 */
func CalculateStatisticalFeatures(content []byte) StatisticalFeatures {
	var sf StatisticalFeatures
	src := string(content)

	// 计算八大统计特征
	sf.LM = roundToSix(float64(lineMax(src)))
	sf.LVC = roundToSix(lineVariationCoefficient(src))
	sf.WM = roundToSix(float64(wordMax(src)))
	sf.WVC = roundToSix(wordVariationCoefficient(src))
	sf.SR = roundToSix(symbolRatio(src))
	sf.TR = roundToSix(tagRatio(src))
	sf.SPL = roundToSix(statementPerLine(src))
	sf.IE = roundToSix(infomationEntropy(src))

	return sf
}

/**
 * @Description: 保留6位小数
 * @author: Mr wpl
 * @param value float64: 值
 * @return float64: 保留6位小数后的值
 */
func roundToSix(value float64) float64 {
	multiplier := math.Pow(10, 6)
	return math.Round(value*multiplier) / multiplier
}

/**
 * @Description: 计算每行字符数
 * @author: Mr wpl
 * @param src string: 内容
 * @return []int64: 每行字符数
 */
func statLines(src string) []int64 {
	var result []int64
	splitResult := strings.Split(src, "\n")
	for _, v := range splitResult {
		result = append(result, int64(len(v)))
	}
	return result
}

/**
 * @Description: 计算每行字符数的最大值
 * @author: Mr wpl
 * @param src string: 内容
 * @return int64: 每行字符数的最大值
 */
func lineMax(src string) int64 {
	lines := stat.IntSlice(statLines(src))
	if len(lines) > 0 {
		result, _ := stat.Max(lines)
		return int64(result)
	}
	return 0
}

/**
 * @Description: 计算每行字符数的变异系数
 * @author: Mr wpl
 * @param src string: 内容
 * @return float64: 每行字符数的变异系数
 */
func lineVariationCoefficient(src string) float64 {
	lines := stat.IntSlice(statLines(src))
	if len(lines) <= 1 || stat.Mean(lines) == 0 {
		return 0.0
	}
	return math.Sqrt(stat.Variance(lines)) / stat.Mean(lines)
}

/**
 * @Description: 计算单词数
 * @author: Mr wpl
 * @param src string: 内容
 * @return []int64: 单词数
 */
func statWords(src string) []int64 {
	var result []int64
	l := int64(0)

	// 按照CloudWalker的方式提取单词长度
	for _, c := range src {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			l++
		} else if l != 0 {
			result = append(result, l)
			l = 0
		}
	}

	if l != 0 {
		result = append(result, l)
	}

	return result
}

/**
 * @Description: 计算单词数的最大值
 * @author: Mr wpl
 * @param src string: 内容
 * @return int64: 单词数的最大值
 */
func wordMax(src string) int64 {
	words := stat.IntSlice(statWords(src))
	if len(words) > 0 {
		result, _ := stat.Max(words)
		return int64(result)
	}
	return 0
}

/**
 * @Description: 计算单词变异系数
 * @author: Mr wpl
 * @param src string: 内容
 * @return float64: 单词变异系数
 */
func wordVariationCoefficient(src string) float64 {
	words := stat.IntSlice(statWords(src))
	if len(words) <= 1 || stat.Mean(words) == 0 {
		return 0.0
	}
	// CloudWalker乘以100
	return math.Sqrt(stat.Variance(words)) / stat.Mean(words) * 100
}

/**
 * @Description: 计算符号比例
 * @author: Mr wpl
 * @param src string: 内容
 * @return float64: 符号比例
 */
func symbolRatio(src string) float64 {
	if len(src) == 0 {
		return 0.0
	}

	// 使用与CloudWalker相同的正则表达式
	symbolReg, _ := regexp.Compile(`[^a-zA-Z0-9]`)
	symbolNumber := len(symbolReg.FindAllString(src, -1))

	return float64(symbolNumber) / float64(len(src)) * 100
}

/**
 * @Description: 计算标签比例
 * @author: Mr wpl
 * @param src string: 内容
 * @return float64: 标签比例
 */
func tagRatio(src string) float64 {
	// 使用与CloudWalker相同的正则表达式
	tagReg, _ := regexp.Compile(`<[\x00-\xFF]*?>`)
	tagNumber := len(tagReg.FindAllString(src, -1))

	words := statWords(src)
	wordCount := float64(len(words))
	if wordCount == 0.0 {
		return 0.0
	}

	return float64(tagNumber) / wordCount * 100
}

/**
 * @Description: 计算语句比例
 * @author: Mr wpl
 * @param src string: 内容
 * @return float64: 语句比例
 */
func statementPerLine(src string) float64 {
	// 使用与CloudWalker相同的正则表达式
	statementReg, _ := regexp.Compile(`;`)
	statementNumber := len(statementReg.FindAllString(src, -1))

	lines := statLines(src)
	lineCount := float64(len(lines))
	if lineCount == 0.0 {
		return 0.0
	}

	return float64(statementNumber) / lineCount
}

/**
 * @Description: 计算信息熵
 * @author: Mr wpl
 * @param src string: 内容
 * @return float64: 信息熵
 */
func infomationEntropy(src string) float64 {
	// 使用与CloudWalker相同的熵计算方法
	var lst []float64
	chrs := 0.00

	// 初始化频率数组
	for i := 0; i < 256; i++ {
		lst = append(lst, 0)
	}

	// 计算字符频率
	for _, chr := range src {
		if 0 <= chr && chr < 256 && chr != '\n' {
			lst[chr]++
			chrs++
		}
	}

	// 计算熵
	var entropy float64
	for i := 0; i < 256; i++ {
		if lst[i] > 0 {
			probability := lst[i] / chrs
			entropy -= probability * math.Log2(probability)
		}
	}

	return entropy
}
