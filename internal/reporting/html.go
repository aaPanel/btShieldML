package reporting

import (
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"crypto/md5"
	"fmt"
	"html"
	"io/ioutil"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type HtmlReporter struct{}

/**
 * @Description: 创建新的HTML报告
 * @author: Mr wpl
 * @return *HtmlReporter: HTML报告
 */
func NewHtmlReporter() *HtmlReporter {
	return &HtmlReporter{}
}

/**
 * @Description: 生成HTML报告
 * @author: Mr wpl
 * @param results []*types.ScanResult: 扫描结果
 * @param outputPath string: 输出路径
 * @return error: 错误
 */
func (r *HtmlReporter) Generate(results []*types.ScanResult, outputPath string) error {
	if outputPath == "" {
		return fmt.Errorf("HTML reporter requires an output path")
	}
	// 创建辅助函数 - 实际集成时应该用真实实现替换这些占位符
	formatFileSize := func(size int64) string {
		const unit = 1024
		if size < unit {
			return fmt.Sprintf("%d B", size)
		}
		div, exp := int64(unit), 0
		for n := size / unit; n >= unit; n /= unit {
			div *= unit
			exp++
		}
		return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
	}

	getMD5Placeholder := func(path string) string {
		return fmt.Sprintf("%x", md5.Sum([]byte(path)))
	}

	// --- 数据处理 ---
	scanTime := time.Now().Format("2006-01-02 15:04:05")
	totalFiles := len(results)
	normalFiles := 0
	suspiciousFiles := 0
	trojanFiles := 0
	errorFiles := 0
	problemFiles := []*types.ScanResult{}

	// 用于统计文件类型分布
	fileTypeStats := make(map[string]int)

	// 用于统计风险分数分布
	riskScoreStats := make(map[string]int)
	riskScoreStats["疑似木马(1级)"] = 0
	riskScoreStats["疑似木马(2级)"] = 0
	riskScoreStats["疑似木马(3级)"] = 0
	riskScoreStats["木马文件(4级)"] = 0
	riskScoreStats["木马文件(5级)"] = 0

	for _, res := range results {
		// 统计文件类型
		fileExt := strings.ToLower(filepath.Ext(res.File.Path))
		if fileExt != "" {
			fileExt = fileExt[1:] // 移除点号
			fileTypeStats[fileExt]++
		} else {
			fileTypeStats["unknown"]++
		}

		if res.Error != nil {
			errorFiles++
			problemFiles = append(problemFiles, res)
			continue
		}

		// 统计风险分数分布
		switch res.OverallRisk {
		case types.RiskNone:
			// 不添加到问题文件列表中
			normalFiles++
		case types.RiskLow:
			suspiciousFiles++
			problemFiles = append(problemFiles, res)
			riskScoreStats["疑似木马(1级)"]++
		case types.RiskMedium:
			suspiciousFiles++
			problemFiles = append(problemFiles, res)
			riskScoreStats["疑似木马(3级)"]++
		case types.RiskHigh:
			trojanFiles++
			problemFiles = append(problemFiles, res)
			riskScoreStats["木马文件(4级)"]++
		case types.RiskCritical:
			trojanFiles++
			problemFiles = append(problemFiles, res)
			riskScoreStats["木马文件(5级)"]++
		default:
			errorFiles++
			problemFiles = append(problemFiles, res)
		}
	}

	// 按风险等级排序：木马文件(Critical) > 疑似木马(High/Medium/Low) > 其他
	sort.Slice(problemFiles, func(i, j int) bool {
		// 定义风险等级优先级
		riskOrder := func(risk types.RiskLevel) int {
			switch risk {
			case types.RiskCritical:
				return 1
			case types.RiskHigh:
				return 2
			case types.RiskMedium:
				return 3
			case types.RiskLow:
				return 4
			default:
				return 5
			}
		}
		return riskOrder(problemFiles[i].OverallRisk) < riskOrder(problemFiles[j].OverallRisk)
	})

	// 转换文件类型统计为JSON格式供图表使用
	var fileTypeLabels []string
	var fileTypeValues []int
	for fileType, count := range fileTypeStats {
		if count > 0 {
			fileTypeLabels = append(fileTypeLabels, fmt.Sprintf(`"%s"`, fileType))
			fileTypeValues = append(fileTypeValues, count)
		}
	}

	// 转换风险分数统计为JSON格式供图表使用
	var riskScoreLabels []string
	var riskScoreValues []int
	var riskScoreColors []string

	// 确保按顺序显示
	riskCategories := []string{"疑似木马(1级)", "疑似木马(2级)", "疑似木马(3级)", "木马文件(4级)", "木马文件(5级)"}
	riskCategoryColors := []string{"#28a745", "#fff5cc", "#ff9900", "#ff3300", "#cc0000"}

	for i, category := range riskCategories {
		if count := riskScoreStats[category]; count > 0 {
			riskScoreLabels = append(riskScoreLabels, fmt.Sprintf(`"%s"`, category))
			riskScoreValues = append(riskScoreValues, count)
			riskScoreColors = append(riskScoreColors, fmt.Sprintf(`"%s"`, riskCategoryColors[i]))
		}
	}
	// --- HTML 生成 ---
	var htmlBuilder strings.Builder

	// 写入 HTML 头部和样式
	htmlBuilder.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>bt-ShieldML 木马查杀报告</title>
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/css/all.min.css">
    <style>
        :root {
            --primary-color: #0070c0;
            --primary-light: #e6f2fc;
            --secondary-color: #f0f0f0;
            --text-color: #333333;
            --light-text: #666666;
            --border-color: #cccccc;
            --risk-low: #f8a532;       /* 橙黄色 - 疑似木马 */
            --risk-medium: #f8a532;     /* 橙黄色 - 疑似木马 */
            --risk-high: #e94747;       /* 鲜红色 - 木马文件 */
            --risk-critical: #e94747;   /* 鲜红色 - 木马文件 */
            --row-hover: rgba(0, 112, 192, 0.1);
            --even-row: #f9f9f9;
            --header-bg: #eaeaea;
            --success-color: #28a745;
        }
        
        body { 
            font-family: 'Arial', 'Microsoft YaHei', sans-serif; 
            background-color: var(--secondary-color); 
            color: var(--text-color); 
            margin: 0; 
            padding: 15px; 
            line-height: 1.5; 
        }
        
        .container { 
            max-width: 1200px; 
            margin: 5px auto; 
            padding: 15px; 
            background-color: #ffffff; 
            border-radius: 8px; 
            box-shadow: 0 2px 10px rgba(0,0,0,0.05); 
        }
        
        h1 { 
            text-align: center; 
            font-size: 24px; 
            font-weight: bold; 
            color: var(--primary-color); 
            margin-bottom: 15px; 
            display: flex;
            align-items: center;
            justify-content: center;
        }
        
        h1 i {
            margin-right: 12px;
            font-size: 28px;
        }
        
        hr { 
            border: none; 
            height: 1px; 
            background-color: var(--border-color); 
            margin-bottom: 25px; 
        }
        
        .timestamp { 
            font-size: 16px; 
            color: var(--light-text); 
            margin-bottom: 25px; 
            text-align: center; 
        }
        
        .charts-container {
            display: none; /* 图表容器不再需要 */
        }
        
        .chart-box {
            display: none; /* 图表盒子不再需要 */
        }
        
        .chart-title {
            font-size: 16px;
            font-weight: bold;
            margin-bottom: 15px;
            color: var(--primary-color);
            display: flex;
            align-items: center;
        }
        
        .chart-title i {
            margin-right: 8px;
        }
        
        .chart-container {
            height: 250px;
            position: relative;
        }
        
        .summary { 
            margin-bottom: 10px; 
            padding: 10px; 
            border: 1px solid var(--border-color); 
            border-radius: 5px; 
            background-color: #ffffff; 
            box-shadow: 0 1px 3px rgba(0,0,0,0.05);
        }
        
        .summary h2 { 
            font-size: 18px; 
            font-weight: bold; 
            color: var(--primary-color); 
            margin-top: 0; 
            margin-bottom: 15px; 
            display: flex;
            align-items: center;
        }
        
        .summary h2 i {
            margin-right: 10px;
            color: var(--primary-color);
        }
        
        .summary ul { 
            list-style: none; 
            padding: 0; 
            margin: 0; 
            display: flex;
            flex-wrap: wrap;
        }
        
        .summary li { 
            font-size: 16px; 
            margin-bottom: 12px; 
            color: var(--text-color); 
            flex-basis: 50%;
            display: flex;
            align-items: center;
        }
        
        .summary li i {
            margin-right: 8px;
            width: 18px;
            text-align: center;
        }
        
        .summary li span { 
            font-weight: bold; 
            color: var(--primary-color);
            margin-left: 5px;
        }
        
        .summary .risk-count {
            margin-top: 10px;
            padding-top: 10px;
            border-top: 1px solid var(--border-color);
            width: 100%;
        }
        
        .file-list h2 { 
            font-size: 18px; 
            font-weight: bold; 
            color: var(--primary-color); 
            margin-top: 0; 
            margin-bottom: 10px; 
            display: flex;
            align-items: center;
        }
        
        .file-list h2 i {
            margin-right: 10px;
        }
        
        .tab-filters {
            display: flex;
            background-color: var(--primary-light);
            border-radius: 8px 8px 0 0;
            border: 1px solid var(--border-color);
            border-bottom: none;
            overflow: hidden;
        }
        
        .tab-btn {
            padding: 8px 15px;
            background-color: transparent;
            border: none;
            border-right: 1px solid var(--border-color);
            cursor: pointer;
            font-size: 14px;
            font-weight: 500;
            color: var(--text-color);
            transition: all 0.2s ease;
            display: flex;
            align-items: center;
        }
        
        .tab-btn:last-child {
            border-right: none;
        }
        
        .tab-btn:hover {
            background-color: rgba(0, 112, 192, 0.1);
        }
        
        .tab-btn.active {
            background-color: var(--primary-color);
            color: white;
        }
        
        .tab-btn .count {
            display: inline-block;
            background-color: rgba(255, 255, 255, 0.3);
            border-radius: 10px;
            padding: 2px 8px;
            font-size: 12px;
            margin-left: 8px;
        }
        
        .tab-btn.active .count {
            background-color: white;
            color: var(--primary-color);
        }
        
        .actions-bar {
            margin: 10px 0;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }
        
        .action-buttons {
            display: flex;
            gap: 10px;
        }
        
        .action-btn {
            padding: 8px 15px;
            background-color: var(--primary-color);
            border: none;
            border-radius: 4px;
            color: white;
            cursor: pointer;
            font-size: 14px;
            display: flex;
            align-items: center;
            transition: background-color 0.2s;
        }
        
        .action-btn:hover {
            background-color: #005ca3;
        }
        
        .action-btn i {
            margin-right: 8px;
        }
        
        .action-btn.danger {
            background-color: var(--risk-high);
        }
        
        .action-btn.danger:hover {
            background-color: #cc2900;
        }
        
        .search-box {
            display: flex;
            align-items: center;
            border: 1px solid var(--border-color);
            border-radius: 4px;
            padding: 5px 10px;
            background-color: white;
        }
        
        .search-box input {
            border: none;
            padding: 5px;
            font-size: 14px;
            outline: none;
            width: 200px;
        }
        
        .search-box i {
            color: var(--light-text);
            margin-right: 5px;
        }
        
        .filters {
            margin-bottom: 15px;
            display: flex;
            justify-content: flex-end;
            align-items: center;
        }
        
        .filter-btn {
            padding: 6px 12px;
            background-color: #ffffff;
            border: 1px solid var(--border-color);
            border-radius: 4px;
            margin-left: 8px;
            cursor: pointer;
            font-size: 14px;
            color: var(--text-color);
            transition: all 0.2s ease;
        }
        
        .filter-btn:hover {
            background-color: var(--row-hover);
        }
        
        .filter-btn.active {
            background-color: var(--primary-color);
            color: white;
            border-color: var(--primary-color);
        }
        
        table { 
            width: 100%; 
            border-collapse: collapse; 
            margin-top: 0; 
            box-shadow: 0 1px 3px rgba(0,0,0,0.05);
            border: 1px solid var(--border-color);
            border-radius: 0 0 8px 8px;
        }
        
        th, td { 
            border: 1px solid var(--border-color); 
            padding: 8px 12px; 
            text-align: left; 
            vertical-align: top; 
        }
        
        th { 
            background-color: var(--header-bg); 
            font-weight: bold; 
            font-size: 14px; 
            color: var(--text-color);
            position: sticky;
            top: 0;
        }
        
        td { 
            font-size: 14px; 
        }
        
        tr:nth-child(even) {
            background-color: var(--even-row);
        }
        
        tr:hover {
            background-color: var(--row-hover);
        }
        
        .risk-indicator {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            width: 100px;
            text-align: center;
            padding: 6px 10px;
            border-radius: 20px;  /* 增加圆角 */
            font-weight: bold;
            font-size: 13px;
            position: relative;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);  /* 添加阴影效果 */
        }
        
        .risk-critical { 
            background: linear-gradient(135deg, #e94747, #c62828);
            color: white;
        }
        
        .risk-high { 
            background: linear-gradient(135deg, #e94747, #c62828);
            color: white;
        }
        
        .risk-medium { 
            background: linear-gradient(135deg, #f8a532, #f57c00);
            color: white;
        }
        
        .risk-low { 
            background: linear-gradient(135deg, #f8a532, #f57c00);
            color: white;
        }
        
        .risk-error { 
            background-color: #e2e3e5; 
            color: #383d41; 
        }
        
        .file-info {
            display: flex;
            flex-direction: column;
        }
        
        .file-path {
            word-break: break-all;
            overflow-wrap: break-word;
            margin-bottom: 5px;
            display: -webkit-box;
            -webkit-line-clamp: 2; 
            -webkit-box-orient: vertical;
            overflow: hidden;
            position: relative;
            font-size: 13px;
        }
        
        .file-path.expanded {
            -webkit-line-clamp: unset;
        }
        
        .path-toggle {
            color: var(--primary-color);
            cursor: pointer;
            font-size: 12px;
            margin-top: 3px;
            display: inline-block;
        }
        
        .file-meta {
            display: flex;
            font-size: 12px;
            color: var(--light-text);
            margin-top: 5px;
        }
        
        .file-meta div {
            margin-right: 15px;
        }
        
        .file-meta i {
            margin-right: 4px;
        }
        
        .details-btn {
            display: inline-block;
            margin-top: 8px;
            padding: 4px 10px;
            border: 1px solid var(--border-color);
            border-radius: 4px;
            background-color: white;
            font-size: 12px;
            cursor: pointer;
            color: var(--primary-color);
            transition: all 0.2s;
        }
        
        .details-btn:hover {
            background-color: var(--primary-light);
            border-color: var(--primary-color);
        }
        
        .details-content {
            margin-top: 10px;
            background-color: var(--primary-light);
            border: 1px solid var(--primary-light);
            border-radius: 4px;
            padding: 10px;
            font-size: 13px;
            position: relative;
        }
        
        .details-content h4 {
            margin: 0 0 10px 0;
            font-size: 14px;
            color: var(--primary-color);
        }
        
        .details-content h5 {
            margin: 10px 0 5px 0;
            font-size: 13px;
        }
        
        .match-rules {
            margin: 10px 0;
        }
        
        .match-rules ul {
            margin: 5px 0;
            padding-left: 20px;
        }
        
        .match-rules li {
            margin-bottom: 3px;
        }
        
        .recommendation {
            background-color: rgba(255, 255, 255, 0.5);
            padding: 8px;
            border-radius: 4px;
            border-left: 3px solid var(--primary-color);
            margin-top: 10px;
        }
        
        .checkbox-container {
            display: flex;
            align-items: center;
        }
        
        .custom-checkbox {
            width: 18px;
            height: 18px;
            border: 1px solid var(--border-color);
            border-radius: 3px;
            margin-right: 10px;
            display: inline-block;
            position: relative;
            cursor: pointer;
            background-color: white;
        }
        
        .custom-checkbox.checked:before {
            content: '✓';
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            color: var(--primary-color);
            font-weight: bold;
        }
        
        .footer { 
            text-align: center; 
            margin-top: 40px; 
            font-size: 12px; 
            color: var(--light-text); 
        }
        
        /* 添加响应式设计 */
        @media (max-width: 768px) {
            .container {
                padding: 15px;
            }
            
            .summary ul {
                flex-direction: column;
            }
            
            .summary li {
                flex-basis: 100%;
            }
            
            .tab-filters {
                flex-direction: column;
            }
            
            .tab-btn {
                border-right: none;
                border-bottom: 1px solid var(--border-color);
            }
            
            .actions-bar {
                flex-direction: column;
                align-items: flex-start;
            }
            
            .action-buttons {
                margin-bottom: 10px;
            }
            
            .filters {
                flex-direction: column;
                align-items: flex-start;
            }
            
            .filter-btn {
                margin-bottom: 8px;
            }
            
            .chart-box {
                flex: 1 1 100%;
            }
            
            .risk-indicator {
                width: 100%;
                justify-content: flex-start;
            }
            
            .risk-score {
                margin-left: auto;
            }
            
            .file-path {
                max-width: 100%;
                -webkit-line-clamp: 1;
            }
        }
        
        /* 模态弹窗样式 */
        .modal-overlay {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background-color: rgba(0, 0, 0, 0.6);
            z-index: 1000;
            align-items: center;
            justify-content: center;
            opacity: 0;
            transition: opacity 0.3s ease;
        }
        
        .modal-overlay.active {
            opacity: 1;
        }
        
        .modal {
            background-color: white;
            border-radius: 12px;
            box-shadow: 0 10px 25px rgba(0, 0, 0, 0.15);
            width: 75%;
            max-width: 900px;
            max-height: 85vh;
            overflow-y: auto;
            padding: 0;
            transform: scale(0.9);
            opacity: 0;
            transition: all 0.3s ease;
        }
        
        .modal.active {
            transform: scale(1);
            opacity: 1;
        }
        
        .modal-header {
            background-color: #f8f9fa;
            padding: 16px 24px;
            border-bottom: 1px solid var(--border-color);
            display: flex;
            align-items: center;
            justify-content: space-between;
            border-radius: 12px 12px 0 0;
        }
        
        .modal-title {
            font-size: 20px;
            font-weight: bold;
            color: var(--primary-color);
            margin: 0;
            display: flex;
            align-items: center;
        }
        
        .modal-title i {
            margin-right: 10px;
            font-size: 22px;
        }
        
        .modal-close {
            background: none;
            border: none;
            font-size: 24px;
            color: #999;
            cursor: pointer;
            transition: color 0.2s;
            width: 30px;
            height: 30px;
            display: flex;
            align-items: center;
            justify-content: center;
            border-radius: 50%;
        }
        
        .modal-close:hover {
            color: var(--primary-color);
            background-color: rgba(0, 112, 192, 0.1);
        }
        
        .modal-body {
            padding: 24px;
        }
        
        .file-details {
            margin-bottom: 30px;
            background-color: #f8f9fa;
            border-radius: 8px;
            padding: 20px;
        }
        
        .file-details h3 {
            font-size: 18px;
            font-weight: 600;
            color: var(--primary-color);
            margin: 0 0 16px 0;
            display: flex;
            align-items: center;
        }
        
        .file-details h3 i {
            margin-right: 8px;
        }
        
        .detail-items {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 15px;
        }
        
        .detail-item {
            margin-bottom: 0;
        }
        
        .detail-label {
            font-weight: 600;
            font-size: 14px;
            color: #666;
            margin-bottom: 5px;
        }
        
        .detail-value {
            word-break: break-all;
            background-color: white;
            padding: 8px 12px;
            border-radius: 4px;
            border: 1px solid #eee;
            font-family: 'Consolas', monospace;
        }
        
        .risk-features, .recommendation {
            margin-bottom: 30px;
        }
        
        .risk-features h3, .recommendation h3 {
            font-size: 18px;
            font-weight: 600;
            color: var(--primary-color);
            margin: 0 0 16px 0;
            display: flex;
            align-items: center;
            border-bottom: 1px solid #eee;
            padding-bottom: 10px;
        }
        
        .risk-features h3 i, .recommendation h3 i {
            margin-right: 8px;
        }
        
        .feature-list {
            background-color: #f8f9fa;
            border-radius: 8px;
            padding: 5px;
        }
        
        .feature-item {
            padding: 12px 15px;
            margin-bottom: 8px;
            background-color: white;
            border-radius: 6px;
            border-left: 4px solid var(--primary-color);
            box-shadow: 0 2px 4px rgba(0,0,0,0.05);
        }
        
        .feature-name {
            font-weight: 600;
            margin-bottom: 6px;
            color: var(--text-color);
            display: flex;
            justify-content: space-between;
        }
        
        .feature-description {
            color: var(--light-text);
            font-size: 14px;
        }
        
        .risk-critical-text {
            color: var(--risk-critical);
            font-weight: 600;
        }
        
        .risk-high-text {
            color: var(--risk-high);
            font-weight: 600;
        }
        
        .risk-medium-text {
            color: var(--risk-medium);
            font-weight: 600;
        }
        
        .risk-low-text {
            color: var(--risk-low);
            font-weight: 600;
        }
        
        .recommendation {
            background-color: #f8f9fa;
            border-radius: 8px;
            padding: 20px;
        }
        
        .recommendation p {
            margin: 0;
            padding: 12px 15px;
            background-color: white;
            border-radius: 6px;
            border-left: 4px solid var(--primary-color);
            color: var(--text-color);
        }
        
        .modal-footer {
            padding: 16px 24px;
            border-top: 1px solid var(--border-color);
            display: flex;
            justify-content: flex-end;
            gap: 12px;
            background-color: #f8f9fa;
            border-radius: 0 0 12px 12px;
        }
        
        .modal-btn {
            padding: 10px 20px;
            border: none;
            border-radius: 6px;
            font-size: 14px;
            font-weight: 500;
            cursor: pointer;
            transition: all 0.2s;
            display: flex;
            align-items: center;
        }
        
        .modal-btn i {
            margin-right: 8px;
        }
        
        .modal-btn-primary {
            background-color: var(--primary-color);
            color: white;
        }
        
        .modal-btn-primary:hover {
            background-color: #005ca3;
        }
        
        .modal-btn-danger {
            background-color: var(--risk-critical);
            color: white;
        }
        
        .modal-btn-danger:hover {
            background-color: #b91c1c;
        }
        
        .modal-btn-default {
            background-color: white;
            color: var(--text-color);
            border: 1px solid var(--border-color);
        }
        
        .modal-btn-default:hover {
            background-color: #f1f1f1;
        }
			
        .report-header {
            display: flex;
            align-items: center;
			justify-content: center; 
            margin-bottom: 20px;
        }
        
        .logo-container {
            display: flex;
            align-items: center;
        }
        
        .report-header h1 {
            margin: 0;
            font-size: 26px;
            font-weight: 600;
            color: var(--primary-color);
        }
        
        .risk-score {
            background-color: rgba(255,255,255,0.3);
            border-radius: 10px;
            padding: 1px 6px;
            font-size: 11px;
            margin-left: 4px;
        }
        
        .risk-score-value {
            font-weight: bold;
            color: var(--text-color);
            background-color: #f8f9fa;
            padding: 4px 10px;
            border-radius: 20px;  /* 增加圆角 */
            display: inline-block;
            min-width: 40px;
            text-align: center;
            box-shadow: 0 1px 2px rgba(0,0,0,0.05);  /* 添加轻微阴影 */
        }
        
        .risk-score-value[data-score="5"], .risk-score-value[data-score="4"] {
            color: white;
            background: linear-gradient(135deg, #e94747, #c62828);  /* 木马文件渐变色 */
        }
        
        .risk-score-value[data-score="3"], .risk-score-value[data-score="2"], .risk-score-value[data-score="1"] {
            color: white;
            background: linear-gradient(135deg, #f8a532, #f57c00);  /* 疑似木马渐变色 */
        }
        
        .risk-level {
            background-color: rgba(255,255,255,0.3);
            border-radius: 10px;
            padding: 1px 6px;
            font-size: 11px;
            margin-left: 4px;
        }
        
        .risk-level-description {
            margin-top: 10px;
            background-color: #f8f9fa;
            border-radius: 8px;
            padding: 10px;
            border: 1px solid var(--border-color);
        }
        
        .risk-level-description h3 {
            font-size: 16px;
            margin-top: 0;
            margin-bottom: 10px;
            color: var(--primary-color);
        }
        
        .risk-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 15px;
            font-size: 14px;
            border-radius: 8px;
            overflow: hidden;
            box-shadow: 0 2px 4px rgba(0,0,0,0.05);
        }
        
        .risk-table th,
        .risk-table td {
            padding: 10px 15px;
            border: 1px solid #dee2e6;
            text-align: left;
        }
        
        .risk-table th {
            background-color: #f8f9fa;
            font-weight: 600;
            color: var(--primary-color);
        }
        
        .risk-table tr:hover {
            background-color: rgba(0, 112, 192, 0.05);
        }
        
        .risk-level-badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;  /* 增加圆角 */
            font-weight: bold;
            color: white;
            font-size: 13px;
            min-width: 80px;
            text-align: center;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);  /* 添加阴影效果 */
        }
        
        .risk-level-badge.risk-critical {
            background: linear-gradient(135deg, #e94747, #c62828);  /* 木马文件渐变色 */
        }
        
        .risk-level-badge.risk-high {
            background: linear-gradient(135deg, #e94747, #c62828);  /* 木马文件渐变色 */
        }
        
        .risk-level-badge.risk-medium {
            background: linear-gradient(135deg, #f8a532, #f57c00);  /* 疑似木马渐变色 */
            color: white;  /* 确保文字为白色 */
        }
        
        .risk-level-badge.risk-low {
            background: linear-gradient(135deg, #f8a532, #f57c00);  /* 疑似木马渐变色 */
            color: white;  /* 确保文字为白色 */
        }
        
        .risk-level-badge.risk-none {
            background-color: var(--success-color);
        }

        /* 移除悬浮提示黑框 */
        [data-tooltip]:before {
            display: none !important;
        }

        [data-tooltip]:after {
            display: none !important;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="report-header">
            <div class="logo-container">
                <img src="data:image/x-icon;base64,AAABAAEAICAAAAEAIACoEAAAFgAAACgAAAAgAAAAQAAAAAEAIAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAANIkfEjCHHFY8pSNWQKcmEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABOmj0AMIYbOi6FGaMshRjvLIUY/zmkIP86pSDvO6Uhoz2mIzpbs0UAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAMIYcNC2FGbsshBj9LIUY/yyFGP8shRj/OaQg/zqlIP86pSD/OqQg/TqlIbk9piQyAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAANIkgCC6GGYsshRj7LIUY/yyFGP8shRj/LIUY/yyFGP85pCD/OqUg/zqlIP86pSD/OqUg/zqlIPs7pSGLQagpCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAADSJHxIuhRm/LIUX/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/zmkIP86pSD/OqUg/zqlIP86pSD/OqUg/zmlIP87pSG/QKcmEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA2iiMILoUZvyyFF/8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/OaQg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP87pSC/Q6gqCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAC6GGn4shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP85pCD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqkH/87pSJ+AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABzrmQCLYUZ2yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/zmkIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIdt5wGcCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAHGtYwIshRjdLIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/OaQg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUh3XjBZgIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAb6thAiyFGN0shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP85pCD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSDddsBjAgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABqqFwALIUY3SyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/zmkIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIN1wvV4AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAGKkUgAshRjbLIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/OaQg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg22u7VwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVZxFACyFGNsshRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP85pCD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSDbYLZKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABMlzsALIUY2yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/zmkIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlINtXskAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEGRLwAshRjbLIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/OaQg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg206uNgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAM4gfACyFGNsshRj/LIUY/yyFGP8shRj/LIUY/yyFGP8shRj/LIUY/yyFGP85pCD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIP86pSDbQKgnAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAArhBcAK4QX2yyEF/8vixnrLYUY6SyFGP8shRj/LIUY/yyFGP8shRj/LIUY/zmkIP86pSD/OqUg/zqlIP86pSD/OqUg/zqlIOc3nh7rOqUg/zmlH9s5pR8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAthRmrLIQY9zihH6UwhxwYLIUY8SyFGP8viRrtLIUY+SyFGP8shRj/OaQg/zqlIP86pSD5OaEg7TmkH/86pSDxPaYjFi6IGKc6pSD3OqUhqQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/kCwGAAAAAD+PLAIzhx8WPKIkHiyFGA4shRjlLIYY/zqlIIEwhhsqLIUY+yyFGP85pCD/OqUg+zymIyorhBiDOaMf/zqlIOU5pCAMNIwfHkCpJxZLrDICAAAAAEqrMwYAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAMIccNC2FGPEuhRm/LoYZdDGHHCw1iiEEAAAAADWKIQgxiRxEPKYjKi6FGBgshRj3LIUY/zmkIP86pSD3OqUgGC6FGyo8oyJEQagoBgAAAABCqCgEPaYkKjulIXQ7pSG/OqUg8TymIzQAAAAAAAAAAAAAAAAAAAAAAAAAADKIHhQthRjZLYUZyyyFGPEshRj/LIUY/y2FGNsuhRmXLoYaTjOJIBIAAAAAOI0lAjGHHCguhhpsO6Uiaj2mIyhGqSwCAAAAAD+nJhI7pSFOO6UhlzqlINs6pSD/OqUg/zqlIPE6pSDLOqUg2T6nJRQAAAAAAAAAAAAAAAAAAAAAM4gfIDCHHDxUm0EANIkgEjCGGkwthRmTLYUZ1yyFGP0shRf/LIUY8S2FGbkuhhpwMIccKkKRLgJLrTUCPaYjKjulIXA6pSG5OqUg8TmlH/86pCD9OqUg1zulIZM8piJMQKgnEl22SAA9piM8P6cmHgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA0iR8KO40oBgAAAAA3iyMEMYcdJC+GGmothRmxLIUY8yyFGP8shRj7LYUZ2zqlINs6pSD7OqUf/zqlIPE6pSCxPKUiaj2mJCRCqCkEAAAAAEerLwZAqCYKAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAMYccEi2FGMcshRjxLYUZsS+GGmgwhxwkN4oiBAAAAAA7jSgIMIcbQC2FGYkthRjnOqQg5zulIYc9piNAR6ouCAAAAABCqCkEPaYjIjulImg6pSGvOqUg8TqlIMc+piQSAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAECRLQguhhqvLoYZgy2FGZ8thRjjLIUY/yyFGP0thRjVLoUZky+GG0oziB8SWKBLAC+GGn47pSF+ZblPAECoJxI8piJIO6UhkzqlINU6pCD9OqUg/zqlIOM6pSGfO6UhgTulIa9LrTUIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAUZo/ADeLJAgAAAAAAAAAADWJIAgwhxw6LoYahy2FGc8shRj7LIUY/yyFGPEthRm7L4YaeDulIng6pSC7OqUg8TqlIP86pCD7OqUhzzulIYU8piM4QagnCAAAAAAAAAAAQ6kqCFqyRgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAO4wmAjKIHiIvhhpqLYUYtyyFGPcshRf/OaQf/zqlIPc6pSC3O6Uiaj2mJSBFqi0CAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAANIkgDi2FGdM6pSDTQKcnDAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAALoYZnzqkIZ8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAvhhpEO6UiRAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA////////////+B///+AH//+AAf//AAD//gAAf/4AAH/8AAA//AAAP/wAAD/8AAA//AAAP/wAAD/8AAA//AAAP/wAAD/8AAA//EACP//EI//z/D/P4D/8B/4H4H//4Af/+Pw/H/AP8A//gYH///gf///+f////n////////////8="
        width="48" height="48" style="vertical-align:middle; margin-right:15px;">
            </div>
            <h1>bt-ShieldML 木马查杀报告</h1>
        </div>
        <hr>
        <div class="timestamp"><i class="far fa-clock"></i> 检测时间：` + scanTime + `</div>

        <div class="summary">
            <h2><i class="fas fa-chart-pie"></i>检测数据汇总</h2>
            <ul>
                <li><i class="fas fa-file"></i>检测文件总数：<span>` + fmt.Sprintf("%d", totalFiles) + `</span></li>
                <li><i class="fas fa-check-circle"></i>正常文件量：<span>` + fmt.Sprintf("%d", normalFiles) + `</span></li>
                <li class="risk-count"><i class="fas fa-exclamation-triangle"></i>疑似木马文件数量：<span>` + fmt.Sprintf("%d", suspiciousFiles) + `</span></li>
                <li class="risk-count"><i class="fas fa-radiation"></i>木马文件数量：<span>` + fmt.Sprintf("%d", trojanFiles) + `</span></li>
            </ul>
            
            <div class="risk-level-description">
                <h3>风险评分标准</h3>
                <table class="risk-table">
                    <tr>
                        <th>风险分数</th>
                        <th>风险等级</th>
                        <th>描述</th>
                    </tr>
                    <tr>
                        <td><span class="risk-score-value" data-score="4-5">4~5级</span></td>
                        <td><span class="risk-level-badge risk-critical">木马文件</span></td>
                        <td>确认为恶意代码，建议立即处理</td>
                    </tr>
                    <tr>
                        <td><span class="risk-score-value" data-score="1-3">1~3级</span></td>
                        <td><span class="risk-level-badge risk-low">疑似木马</span></td>
                        <td>包含可疑代码特征，建议审查</td>
                    </tr>
                </table>
            </div>
        </div>

        <div class="file-list">
            <h2><i class="fas fa-list"></i>检测文件结果列表</h2>
            
            <div class="tab-filters">
                <button class="tab-btn active" data-filter="all">全部<span class="count">` + fmt.Sprintf("%d", len(problemFiles)) + `</span></button>
                <button class="tab-btn" data-filter="critical">木马文件<span class="count">` + fmt.Sprintf("%d", trojanFiles) + `</span></button>
                <button class="tab-btn" data-filter="suspicious">疑似木马<span class="count">` + fmt.Sprintf("%d", suspiciousFiles) + `</span></button>
                <button class="tab-btn" data-filter="error">扫描错误<span class="count">` + fmt.Sprintf("%d", errorFiles) + `</span></button>
            </div>
            
            <div class="actions-bar">
                <div class="action-buttons">
                    <button class="action-btn" id="exportPdfBtn"><i class="fas fa-file-pdf"></i>导出 PDF</button>
                    <button class="action-btn" id="exportExcelBtn"><i class="fas fa-file-excel"></i>导出 Excel</button>
                </div>
                <div class="search-box">
                    <i class="fas fa-search"></i>
                    <input type="text" id="searchInput" placeholder="搜索文件名或路径...">
                </div>
            </div>
            
            <div class="filters">
                <button class="filter-btn" data-sort="risk">风险优先</button>
                <button class="filter-btn" data-sort="path">路径排序</button>
            </div>
            
            <table id="fileTable">
                <thead>
                    <tr>
                        <th width="3%"><div class="checkbox-container"><div class="custom-checkbox" id="selectAllCheckbox"></div></div></th>
                        <th width="20%">文件名</th>
                        <th width="42%">文件路径</th>
                        <th width="5%">分数</th>
                        <th width="20%">风险等级</th>
                        <th width="10%">操作</th>
                    </tr>
                </thead>
                <tbody>
`)

	if len(problemFiles) > 0 {
		for i, res := range problemFiles {
			// 根据风险等级设置不同的信息
			riskClass := "risk-unknown"
			riskIcon := "fas fa-question-circle"
			dataFilter := "unknown"
			recommendation := "建议在隔离环境中分析此文件，确认是否为恶意代码。"
			riskLevel := int(res.OverallRisk) // 风险级别数值
			riskDesc := "未知"                  // 风险等级描述 - 这个变量会在下方使用

			// 格式化文件大小
			fileSize := formatFileSize(res.File.Size)
			filePath := html.EscapeString(res.File.Path)
			fileName := filepath.Base(res.File.Path)
			fileName = html.EscapeString(fileName)

			// 模拟 MD5 值，实际实现中应该获取真实值
			fileMD5 := getMD5Placeholder(res.File.Path)

			// 格式化修改时间
			modTime := res.File.ModTime.Format("2006-01-02 15:04:05")

			if res.Error != nil {
				riskDesc = "扫描错误"
				riskClass = "risk-error"
				riskIcon = "fas fa-exclamation-circle"
				dataFilter = "error"
				recommendation = "请检查文件权限和完整性，或尝试重新扫描。"
				riskLevel = 0
			} else {
				// 根据风险等级设置不同的信息
				switch res.OverallRisk {
				case types.RiskCritical: // 5
					riskDesc = "木马文件"
					riskClass = "risk-critical"
					riskIcon = "fas fa-skull-crossbones"
					dataFilter = "critical"
					recommendation = "强烈建议立即删除此文件或将其隔离，并检查系统是否已被入侵。"
				case types.RiskHigh: // 4
					riskDesc = "木马文件"
					riskClass = "risk-high"
					riskIcon = "fas fa-exclamation-triangle"
					dataFilter = "critical"
					recommendation = "建议将此文件隔离，并进行深入安全分析。"
				case types.RiskMedium: // 3
					riskDesc = "疑似木马"
					riskClass = "risk-medium"
					riskIcon = "fas fa-exclamation-triangle"
					dataFilter = "suspicious"
					recommendation = "建议将此文件隔离，并进行安全审核。"
				case types.RiskLow: // 2
					riskDesc = "疑似木马"
					riskClass = "risk-low"
					riskIcon = "fas fa-exclamation-triangle"
					dataFilter = "suspicious"
					recommendation = "建议关注此文件的行为，必要时进行代码审查。"
				default:
					riskDesc = "未知"
					riskClass = "risk-unknown"
					riskIcon = "fas fa-question-circle"
					dataFilter = "unknown"
				}
			}

			// 获取风险分数 (1-5)
			riskScore := riskLevel

			htmlBuilder.WriteString(fmt.Sprintf(`
					<tr data-filter="%s" data-risk="%d" data-filename="%s" data-id="%d">
						<td><div class="checkbox-container"><div class="custom-checkbox file-checkbox"></div></div></td>
						<td>%s</td>
                        <td><div class="file-path">%s</div><span class="path-toggle">查看更多</span></td>
						<td><span class="risk-score-value" data-score="%d">%d级</span></td>
						<td><span class="risk-indicator %s"><i class="%s"></i>%s</span></td>
						<td>
							<button class="details-btn" onclick="showModal(%d)">详情</button>
						</td>
                    </tr>
			`, dataFilter, int(res.OverallRisk), fileName, i, fileName, filePath, riskScore, riskScore, riskClass, riskIcon, riskDesc, i))

			// 生成每个文件的模态弹窗内容
			var findingsHTML strings.Builder
			if len(res.Findings) > 0 {
				for _, finding := range res.Findings {
					findingsHTML.WriteString(fmt.Sprintf(`
						<div class="feature-item">
							<div class="feature-name">%s <span class="risk-%s-text">(%s)</span></div>
							<div class="feature-description">%s</div>
						</div>
					`, finding.AnalyzerName, strings.ToLower(finding.Risk.String()), finding.Risk.String(), html.EscapeString(finding.Description)))
				}
			} else {
				findingsHTML.WriteString(`<div class="feature-item">未检测到特定特征</div>`)
			}

			// 添加详细的模态弹窗HTML
			htmlBuilder.WriteString(fmt.Sprintf(`
				<div class="modal-content" id="modal-content-%d" style="display:none">
					<div class="file-details">
						<h3><i class="fas fa-file-alt"></i>文件基本信息</h3>
						<div class="detail-items">
							<div class="detail-item">
								<div class="detail-label">文件名称</div>
								<div class="detail-value">%s</div>
							</div>
							<div class="detail-item">
								<div class="detail-label">文件大小</div>
								<div class="detail-value">%s</div>
							</div>
							<div class="detail-item">
								<div class="detail-label">修改时间</div>
								<div class="detail-value">%s</div>
							</div>
							<div class="detail-item">
								<div class="detail-label">MD5值</div>
								<div class="detail-value">%s</div>
							</div>
							<div class="detail-item" style="grid-column: 1 / -1;">
								<div class="detail-label">文件路径</div>
								<div class="detail-value">%s</div>
							</div>
							<div class="detail-item">
								<div class="detail-label">风险分数</div>
								<div class="detail-value"><span class="risk-score-value" data-score="%d">%d级</span></div>
							</div>
							<div class="detail-item">
								<div class="detail-label">风险等级</div>
								<div class="detail-value"><span class="risk-indicator %s" style="width:auto; display:inline-flex;"><i class="%s"></i>%s</span></div>
							</div>
						</div>
					</div>
				
					
					<div class="recommendation">
						<h3><i class="fas fa-lightbulb"></i>处理建议</h3>
						<p>%s</p>
					</div>
				</div>
			`, i, fileName, fileSize, modTime, fileMD5, filePath, riskScore, riskScore, riskClass, riskIcon, riskDesc, recommendation))
		}
	} else {
		htmlBuilder.WriteString(`<tr><td colspan="5" style="text-align:center; color: #6c757d;">未发现问题文件</td></tr>`)
	}

	// --- HTML 结尾和写入文件 ---
	htmlBuilder.WriteString(`
                </tbody>
            </table>
        </div>

        <div class="footer">
            &copy; ` + fmt.Sprintf("%d", time.Now().Year()) + ` bt-ShieldML. All rights reserved.
        </div>
    </div>
			
			<!-- 模态弹窗 -->
			<div class="modal-overlay" id="modal-overlay">
				<div class="modal" id="modal-container">
					<div class="modal-header">
						<h3 class="modal-title"><i class="fas fa-file-search"></i>文件详情分析</h3>
						<button class="modal-close" onclick="closeModal()">&times;</button>
					</div>
					<div class="modal-body" id="modal-body">
						<!-- 动态内容将在这里加载 -->
					</div>
					<div class="modal-footer">
						<button class="modal-btn modal-btn-default" onclick="closeModal()"><i class="fas fa-times"></i>关闭</button>
					</div>
				</div>
			</div>
			
			<script src="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/5.15.4/js/all.min.js"></script>
			<script src="https://cdnjs.cloudflare.com/ajax/libs/jspdf/2.5.1/jspdf.umd.min.js"></script>
			<script src="https://cdnjs.cloudflare.com/ajax/libs/xlsx/0.18.5/xlsx.full.min.js"></script>
			<script>
				// 初始化所有功能
				document.addEventListener('DOMContentLoaded', function() {
					// 表格筛选和排序功能
					const table = document.getElementById('fileTable');
					const rows = Array.from(table.querySelectorAll('tbody tr'));
					const tabBtns = document.querySelectorAll('.tab-btn');
					const sortBtns = document.querySelectorAll('.filter-btn[data-sort]');
					const searchInput = document.getElementById('searchInput');
					
					// 筛选功能 - 选项卡
					tabBtns.forEach(btn => {
						btn.addEventListener('click', () => {
							const filter = btn.getAttribute('data-filter');
							
							// 更新按钮状态
							tabBtns.forEach(b => b.classList.remove('active'));
							btn.classList.add('active');
							
							// 筛选行
							rows.forEach(row => {
								if(filter === 'all' || row.getAttribute('data-filter') === filter) {
									row.style.display = '';
								} else {
									row.style.display = 'none';
								}
							});
						});
					});
					
					// 排序功能
					sortBtns.forEach(btn => {
						btn.addEventListener('click', () => {
							const sort = btn.getAttribute('data-sort');
							const tbody = table.querySelector('tbody');
							
							// 更新按钮状态
							sortBtns.forEach(b => b.classList.remove('active'));
							btn.classList.add('active');
							
							// 排序行
							const sortedRows = rows.slice();
							
							if(sort === 'risk') {
								sortedRows.sort((a, b) => {
									return parseInt(a.getAttribute('data-risk')) > 
										parseInt(b.getAttribute('data-risk')) ? 1 : -1;
								});
							} else if(sort === 'path') {
								sortedRows.sort((a, b) => {
									return a.getAttribute('data-filename').localeCompare(
										b.getAttribute('data-filename'));
								});
							}
							
							// 重新添加排序后的行
							sortedRows.forEach(row => tbody.appendChild(row));
						});
					});
					
					// 搜索功能
					searchInput.addEventListener('input', () => {
						const searchTerm = searchInput.value.toLowerCase();
						
						rows.forEach(row => {
							const filename = row.getAttribute('data-filename').toLowerCase();
							if (filename.includes(searchTerm)) {
								row.style.display = '';
							} else {
								row.style.display = 'none';
							}
						});
					});
					
					// 全选/全不选功能
					const selectAllCheckbox = document.getElementById('selectAllCheckbox');
					const fileCheckboxes = document.querySelectorAll('.file-checkbox');
					
					selectAllCheckbox.addEventListener('click', () => {
						const isChecked = selectAllCheckbox.classList.contains('checked');
						
						if (isChecked) {
							selectAllCheckbox.classList.remove('checked');
							fileCheckboxes.forEach(checkbox => {
								checkbox.classList.remove('checked');
							});
						} else {
							selectAllCheckbox.classList.add('checked');
							fileCheckboxes.forEach(checkbox => {
								checkbox.classList.add('checked');
							});
						}
					});
					
					fileCheckboxes.forEach(checkbox => {
						checkbox.addEventListener('click', (e) => {
							e.stopPropagation();
							checkbox.classList.toggle('checked');
							
							// 检查是否所有文件都被选中
							const allChecked = Array.from(fileCheckboxes).every(cb => 
								cb.classList.contains('checked'));
							
							if (allChecked) {
								selectAllCheckbox.classList.add('checked');
							} else {
								selectAllCheckbox.classList.remove('checked');
							}
						});
					});
					
					// 导出PDF功能
					document.getElementById('exportPdfBtn').addEventListener('click', exportToPDF);
					
					// 导出Excel功能
					document.getElementById('exportExcelBtn').addEventListener('click', exportToExcel);
					
					// 添加路径切换功能
					document.querySelectorAll('.path-toggle').forEach(toggle => {
						toggle.addEventListener('click', function() {
							const filePath = this.previousElementSibling;
							if (filePath.classList.contains('expanded')) {
								filePath.classList.remove('expanded');
								this.textContent = '查看更多';
							} else {
								filePath.classList.add('expanded');
								this.textContent = '收起';
							}
						});
					});

					// 更新风险分数值的颜色
					document.querySelectorAll('.risk-score-value').forEach(el => {
						const score = parseInt(el.getAttribute('data-score'));
						el.setAttribute('data-score', score);
					});
				});
				
				// 弹窗相关函数
				function showModal(id) {
					const modalOverlay = document.getElementById('modal-overlay');
					const modal = document.getElementById('modal-container');
					const modalBody = document.getElementById('modal-body');
					const contentElement = document.getElementById('modal-content-' + id);
					
					// 复制内容到模态框
					modalBody.innerHTML = '';
					if (contentElement) {
						modalBody.innerHTML = contentElement.innerHTML;
					}
					
					// 显示模态框并添加活动类
					modalOverlay.style.display = 'flex';
					
					// 强制浏览器重绘
					void modalOverlay.offsetWidth;
					
					// 添加活动类以触发动画
					modalOverlay.classList.add('active');
					modal.classList.add('active');
					
					// 阻止事件冒泡
					modal.onclick = function(e) {
						e.stopPropagation();
					};
					
					// 点击遮罩层关闭模态框
					modalOverlay.onclick = function(e) {
						if (e.target === modalOverlay) {
							closeModal();
						}
					};
					
					// 添加ESC键关闭模态框
					document.addEventListener('keydown', function(e) {
						if (e.key === 'Escape') {
							closeModal();
						}
					});
				}
				
				function closeModal() {
					const modalOverlay = document.getElementById('modal-overlay');
					const modal = document.getElementById('modal-container');
					
					// 移除活动类以触发关闭动画
					modalOverlay.classList.remove('active');
					modal.classList.remove('active');
					
					// 等待动画完成后隐藏模态框
					setTimeout(() => {
						modalOverlay.style.display = 'none';
					}, 300);
				}
				
				// 导出PDF功能
				function exportToPDF() {
					// 实际实现时应该使用jsPDF库生成PDF
					alert('导出PDF功能尚未实现，此功能将允许导出完整的检测报告为PDF文件。');
				}
				
				// 导出Excel功能
				function exportToExcel() {
					// 实际实现时应该使用xlsx库导出Excel
					alert('导出Excel功能尚未实现，此功能将允许导出文件列表和检测结果为Excel文件。');
				}
			</script>
</body>
</html>
`)

	htmlContent := htmlBuilder.String()
	err := ioutil.WriteFile(outputPath, []byte(htmlContent), 0644)
	if err != nil {
		logging.ErrorLogger.Printf("Failed to write HTML report to %s: %v", outputPath, err)
		return fmt.Errorf("failed to write HTML report: %w", err)
	}

	return nil
}
