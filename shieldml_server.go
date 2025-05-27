package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type ScanResult struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	MD5       string `json:"md5"`
	SHA256    string `json:"sha256"`
	Type      string `json:"type"`
	Risk      string `json:"risk"`      // 风险说明（如“疑似木马”）
	RiskScore int    `json:"riskScore"` // 风险分数（如1、2、3、4、5、0）
	Icon      string `json:"icon"`
	Desc      string `json:"desc"`
}

// JSON文件结构体
type JsonFileData struct {
	Results []struct {
		Filename    string `json:"filename"`
		Type        string `json:"type"`
		Risk        int    `json:"risk"`
		RiskText    string `json:"risk_text"`
		Description string `json:"description"`
	} `json:"results"`
}

// 扫描锁，防止并发扫描
var scanLock sync.Mutex

// 上次扫描时间
var lastScanTime time.Time

// 添加安全相关HTTP头
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 防止目录列表
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// 防止点击劫持
		w.Header().Set("X-Frame-Options", "DENY")
		// XSS保护
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		// 内容安全策略
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://cdn.jsdelivr.net; font-src 'self' https://fonts.gstatic.com https://cdn.jsdelivr.net; img-src 'self' data:;")
		// 不缓存敏感页面
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		next.ServeHTTP(w, r)
	})
}

func main() {
	// API路由
	http.HandleFunc("/api/scan", scanHandler)
	http.HandleFunc("/api/report", reportFalsePositiveHandler)
	http.HandleFunc("/api/detail", fileDetailHandler)

	// 静态文件处理
	fileHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 允许访问API路径
		if strings.HasPrefix(r.URL.Path, "/api/") {
			// API 请求会在单独的处理器中处理，这里不做操作
			http.NotFound(w, r)
			return
		}

		// 允许访问 shieldml_scan.html
		if r.URL.Path == "/shieldml_scan.html" {
			w.Header().Set("Content-Type", "text/html")
			http.ServeFile(w, r, "shieldml_scan.html")
			return
		}

		// 允许访问 shieldml_scan.js
		if r.URL.Path == "/shieldml_scan.js" {
			w.Header().Set("Content-Type", "application/javascript")
			http.ServeFile(w, r, "shieldml_scan.js")
			return
		}

		// 所有其他路径都重定向到 shieldml_scan.html
		http.Redirect(w, r, "/shieldml_scan.html", http.StatusFound)
	})

	// 应用安全中间件
	http.Handle("/", securityMiddleware(fileHandler))

	fmt.Println("服务已启动：http://localhost:6528/shieldml_scan.html")
	http.ListenAndServe(":6528", nil)
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持POST", http.StatusMethodNotAllowed)
		return
	}

	// 防止并发扫描，加锁
	scanLock.Lock()
	defer scanLock.Unlock()

	// 清空临时目录
	tmpDir := "data/tmp"
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		// 目录不存在，创建目录
		err = os.MkdirAll(tmpDir, 0755)
		if err != nil {
			fmt.Printf("创建临时目录失败: %v\n", err)
			http.Error(w, "创建临时目录失败", 500)
			return
		}
	} else {
		// 目录存在，清空目录
		files, err := ioutil.ReadDir(tmpDir)
		if err != nil {
			fmt.Printf("读取临时目录失败: %v\n", err)
			http.Error(w, "读取临时目录失败", 500)
			return
		}
		for _, f := range files {
			os.Remove(filepath.Join(tmpDir, f.Name()))
		}
	}

	// 解析上传的文件
	err := r.ParseMultipartForm(20 << 20) // 20MB
	if err != nil {
		http.Error(w, "文件解析失败", 400)
		return
	}

	files := r.MultipartForm.File["file"]
	var results []ScanResult

	// 临时文件夹，用于存放待检测文件
	tempDir := filepath.Join(os.TempDir(), "shieldml_scan_"+time.Now().Format("20060102150405"))
	err = os.MkdirAll(tempDir, 0755)
	if err != nil {
		http.Error(w, "创建临时目录失败", 500)
		return
	}
	defer os.RemoveAll(tempDir)

	// 保存所有文件到临时文件夹和临时目录
	var filePaths []string
	fileInfos := make(map[string]struct {
		size   int64
		md5    string
		sha256 string
		ftype  string
	})

	for _, fh := range files {
		if !strings.HasSuffix(strings.ToLower(fh.Filename), ".php") {
			continue // 只处理php文件
		}

		file, err := fh.Open()
		if err != nil {
			continue
		}
		defer file.Close()

		// 创建唯一文件名，避免冲突
		tmpPath := filepath.Join(tempDir, fh.Filename)
		out, err := os.Create(tmpPath)
		if err != nil {
			continue
		}

		// 读取文件内容
		fileContent, err := ioutil.ReadAll(file)
		if err != nil {
			out.Close()
			continue
		}

		// 计算MD5
		md5Hash := md5.Sum(fileContent)
		md5Str := hex.EncodeToString(md5Hash[:])

		// 将文件内容写入临时文件
		out.Write(fileContent)
		out.Close()

		// 将文件保存到data/tmp目录，命名为md5.扩展名
		ext := filepath.Ext(fh.Filename)
		tmpFilePath := filepath.Join(tmpDir, md5Str+ext)
		err = ioutil.WriteFile(tmpFilePath, fileContent, 0644)
		if err != nil {
			fmt.Printf("保存临时文件失败: %v\n", err)
		}

		// 重新打开文件以计算SHA256
		file.Seek(0, 0)
		sha256Str, _ := calcSHA256(file)

		filePaths = append(filePaths, tmpPath)
		fileInfos[fh.Filename] = struct {
			size   int64
			md5    string
			sha256 string
			ftype  string
		}{
			size:   int64(len(fileContent)),
			md5:    md5Str,
			sha256: sha256Str,
			ftype:  getFileType(fh.Filename),
		}
	}

	if len(filePaths) == 0 {
		http.Error(w, "没有有效的文件", 400)
		return
	}

	// 设置命令的输出重定向
	cmd := exec.Command("./bt-shieldml", "-path", tempDir, "-format", "json")
	err = cmd.Run()
	if err != nil {
		fmt.Println("检测引擎调用失败:", err)
		http.Error(w, "检测引擎调用失败", 500)
		return
	}

	// 检测完成后，读取json文件
	jsonData, err := readJsonFile("data/webshellJson.json")
	fmt.Println("检测结果:", jsonData)
	if err != nil {
		fmt.Println("读取JSON文件失败:", err)
		http.Error(w, "读取结果失败", 500)
		return
	}

	// 创建映射表，方便快速查找临时目录中的文件路径对应关系
	tempFileMapping := make(map[string]string)
	for _, path := range filePaths {
		tempFileMapping[path] = filepath.Base(path)
	}

	// 只处理当前批次的文件结果
	for _, res := range jsonData.Results {
		// 检查是否是本次扫描的文件
		originalName := ""
		for fullPath, baseName := range tempFileMapping {
			if strings.Contains(res.Filename, fullPath) || res.Filename == baseName {
				originalName = baseName
				break
			}
		}

		// 如果找不到对应的原始文件名，说明不是本批次的结果，跳过
		if originalName == "" {
			continue
		}

		fileInfo, exists := fileInfos[originalName]
		if !exists {
			continue
		}

		// 设置风险等级和图标
		var icon string = "unknown"
		var risk string = "未知"
		var desc string = res.Description
		var riskScore int = res.Risk

		if res.Risk >= 4 {
			icon = "danger"
			risk = res.RiskText
		} else if res.Risk >= 1 {
			icon = "warning"
			risk = res.RiskText
		} else {
			icon = "success"
			risk = "无风险"
		}

		results = append(results, ScanResult{
			Filename:  originalName,
			Size:      fileInfo.size,
			MD5:       fileInfo.md5,
			SHA256:    fileInfo.sha256,
			Type:      fileInfo.ftype,
			Risk:      risk,
			RiskScore: riskScore,
			Icon:      icon,
			Desc:      desc,
		})
	}

	// 在返回结果之前添加排序逻辑
	sort.Slice(results, func(i, j int) bool {
		// 定义风险等级优先级：木马文件 > 疑似木马 > 安全文件 > 其他
		riskOrder := map[string]int{
			"木马文件": 1,
			"疑似木马": 2,
			"无风险":  3,
			"未知":   4,
		}

		// 获取两个文件的风险等级优先级
		orderI := riskOrder[results[i].Risk]
		orderJ := riskOrder[results[j].Risk]

		// 按风险等级优先级排序（从高到低）
		return orderI < orderJ
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"results": results})
}

// 读取JSON文件
func readJsonFile(path string) (*JsonFileData, error) {
	// 确保文件存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("JSON文件不存在: %s", path)
	}

	// 读取文件内容
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 解析JSON
	var data JsonFileData
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func calcHash(path string) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	hMd5 := md5.New()
	hSha := sha256.New()
	io.Copy(io.MultiWriter(hMd5, hSha), f)
	return hex.EncodeToString(hMd5.Sum(nil)), hex.EncodeToString(hSha.Sum(nil))
}

func getFileType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".php":
		return "php"
	case ".jsp":
		return "jsp"
	case ".jspx":
		return "jspx"
	case ".asp":
		return "asp"
	default:
		if len(ext) > 1 {
			return ext[1:] // 安全地移除前导点
		}
		return "unknown"
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// 误报上报处理函数
func reportFalsePositiveHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "仅支持POST", http.StatusMethodNotAllowed)
		return
	}

	// 解析多部分表单，以获取文件
	err := r.ParseMultipartForm(20 << 20) // 20MB
	if err != nil {
		http.Error(w, "文件解析失败", 400)
		return
	}

	// 获取文件
	file, header, err := r.FormFile("file")
	if err != nil {
		fmt.Printf("获取文件失败: %v\n", err)
		http.Error(w, "获取文件失败", 400)
		return
	}
	defer file.Close()

	// 获取文件信息
	filename := header.Filename
	fileType := r.FormValue("type")
	if fileType == "" {
		fileType = getFileType(filename)
	}

	// 获取风险分数
	riskLevel := r.FormValue("type")
	if riskLevel == "" {
		riskLevel = "0" // 默认风险等级
	}

	md5Hash := r.FormValue("md5")

	// 保存文件到临时目录
	tempFile, err := ioutil.TempFile("", "report-*."+fileType)
	if err != nil {
		fmt.Printf("创建临时文件失败: %v\n", err)
		http.Error(w, "创建临时文件失败", 500)
		return
	}
	defer os.Remove(tempFile.Name())

	// 复制文件内容
	_, err = io.Copy(tempFile, file)
	if err != nil {
		fmt.Printf("复制文件内容失败: %v\n", err)
		http.Error(w, "复制文件内容失败", 500)
		return
	}
	tempFile.Close()

	// 如果没有提供MD5，计算文件的MD5
	if md5Hash == "" {
		md5Hash, _ = calcHash(tempFile.Name())
	}

	// 获取access_key
	accessKey, _ := getUserAccessKey()

	// 获取token
	tokenInfo, err := getReportToken(tempFile.Name(), accessKey)
	if err != nil {
		fmt.Printf("获取上报token失败: %v\n", err)
		http.Error(w, "获取上报token失败", 500)
		return
	}

	// 从响应中提取token
	var token string
	if resMap, ok := tokenInfo.Res.(map[string]interface{}); ok {
		if tokenStr, ok := resMap["token"].(string); ok {
			token = tokenStr
		}
	}

	if token == "" {
		fmt.Println("无法从响应中提取token")
		http.Error(w, "获取token失败", 500)
		return
	}

	// 构建上报参数
	params := map[string]string{
		"auto":       "1",
		"token":      token,
		"access_key": accessKey,
	}

	// 根据风险分数设置class和type
	riskInt := 0
	fmt.Sscanf(riskLevel, "%d", &riskInt)

	if riskInt == 0 {
		params["class"] = "0" // 白样本
		params["type"] = "0"  // 0风险
	} else if riskInt >= 4 {
		params["class"] = "1"                       // 黑样本
		params["type"] = fmt.Sprintf("%d", riskInt) // 使用原始风险等级
	} else {
		params["class"] = "1"                       // 疑似木马也作为黑样本
		params["type"] = fmt.Sprintf("%d", riskInt) // 使用原始风险等级
	}

	// 上传文件 - 直接使用原始文件
	err = uploadFalsePositive(tempFile.Name(), filename, fileType, md5Hash, params)
	if err != nil {
		fmt.Printf("上报误报失败: %v\n", err)
		http.Error(w, "上报误报失败", 500)
		return
	}

	// 返回成功
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "误报上报成功",
	})
}

// TokenResponse 结构体
type TokenResponse struct {
	Success bool        `json:"success"`
	Res     interface{} `json:"res"`
	Nonce   int64       `json:"nonce"`
}

// 获取上报token
func getReportToken(filePath, accessKey string) (*TokenResponse, error) {
	// 构建请求URL
	apiUrl := "https://www.bt.cn/api/v2/error/information"

	// 创建一个缓冲区来存储请求体
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// 添加文件
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("创建表单文件失败: %v", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("复制文件内容失败: %v", err)
	}

	// 添加其他表单字段
	writer.WriteField("access_key", accessKey)
	writer.WriteField("auto", "1")
	writer.WriteField("class", "2")
	writer.WriteField("type", "0")

	// 关闭writer以完成请求体
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("关闭writer失败: %v", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", apiUrl, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置Content-Type
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	fmt.Println("响应:", string(body))

	// 解析响应
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if !tokenResp.Success {
		errMsg, ok := tokenResp.Res.(string)
		if ok {
			return nil, fmt.Errorf("获取token失败: %s", errMsg)
		}
		return nil, fmt.Errorf("获取token失败")
	}

	return &tokenResp, nil
}

// 获取用户access_key
func getUserAccessKey() (string, error) {
	// 读取用户信息文件
	userInfoPath := "/www/server/panel/data/userInfo.json"

	// 如果在开发环境，可以使用相对路径
	if !fileExists(userInfoPath) {
		userInfoPath = "userInfo.json" // 尝试当前目录
	}

	if !fileExists(userInfoPath) {
		return "", fmt.Errorf("用户信息文件不存在")
	}

	content, err := ioutil.ReadFile(userInfoPath)
	if err != nil {
		return "", err
	}

	var userInfo struct {
		AccessKey string `json:"access_key"`
	}

	if err := json.Unmarshal(content, &userInfo); err != nil {
		return "", err
	}

	if userInfo.AccessKey == "" {
		return "", fmt.Errorf("access_key为空")
	}

	return userInfo.AccessKey, nil
}

// 上传误报文件
func uploadFalsePositive(filePath, filename, fileType, md5Hash string, params map[string]string) error {
	// 构建请求URL
	apiUrl := "https://www.bt.cn/api/v2/error/information"

	// 创建一个缓冲区来存储请求体
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// 添加文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %v", err)
	}
	defer file.Close()

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("创建表单文件失败: %v", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("复制文件内容失败: %v", err)
	}

	// 添加其他表单字段 - 确保包含所有必要字段
	writer.WriteField("access_key", params["access_key"])
	writer.WriteField("token", params["token"])
	writer.WriteField("type", params["type"])   // 风险级别
	writer.WriteField("class", params["class"]) // 样本类型
	writer.WriteField("auto", params["auto"])   // 上报类型

	// 关闭writer以完成请求体
	err = writer.Close()
	if err != nil {
		return fmt.Errorf("关闭writer失败: %v", err)
	}

	// 创建请求
	req, err := http.NewRequest("POST", apiUrl, &requestBody)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置Content-Type
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	fmt.Println("上报响应:", string(body))

	// 检查响应
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("上报失败，状态码: %d", resp.StatusCode)
	}

	return nil
}

// 计算SHA256
func calcSHA256(reader io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// 文件详情处理函数
func fileDetailHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "仅支持GET", http.StatusMethodNotAllowed)
		return
	}

	// 获取文件MD5和类型
	md5Hash := r.URL.Query().Get("md5")
	fileType := r.URL.Query().Get("type")
	mode := r.URL.Query().Get("mode")

	if md5Hash == "" || fileType == "" {
		http.Error(w, "参数不完整", 400)
		return
	}

	// 构建文件路径
	filePath := filepath.Join("data/tmp", md5Hash+"."+fileType)

	// 检查文件是否存在
	if !fileExists(filePath) {
		http.Error(w, "文件不存在", 404)
		return
	}

	// 读取文件内容
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		http.Error(w, "读取文件失败", 500)
		return
	}

	// 返回文件内容（使用base64编码确保二进制安全）
	w.Header().Set("Content-Type", "application/json")
	if mode == "base64" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"content":  base64.StdEncoding.EncodeToString(content),
			"md5":      md5Hash,
			"type":     fileType,
			"isBase64": true,
		})
	} else {
		// 默认文本模式
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"content":  string(content),
			"md5":      md5Hash,
			"type":     fileType,
			"isBase64": false,
		})
	}
}
