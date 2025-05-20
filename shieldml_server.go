package main

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MD5      string `json:"md5"`
	SHA256   string `json:"sha256"`
	Type     string `json:"type"`
	Risk     string `json:"risk"`
	Icon     string `json:"icon"`
	Desc     string `json:"desc"`
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

func main() {
	http.HandleFunc("/api/scan", scanHandler)
	http.Handle("/", http.FileServer(http.Dir(".")))

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

	// 保存所有文件到临时文件夹
	var filePaths []string
	fileInfos := make(map[string]struct {
		size   int64
		md5    string
		sha256 string
		ftype  string
	})

	for _, fh := range files {
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
		size, _ := io.Copy(out, file)
		out.Close()

		filePaths = append(filePaths, tmpPath)
		md5Str, sha256Str := calcHash(tmpPath)

		fileInfos[fh.Filename] = struct {
			size   int64
			md5    string
			sha256 string
			ftype  string
		}{
			size:   size,
			md5:    md5Str,
			sha256: sha256Str,
			ftype:  getFileType(fh.Filename),
		}
	}

	if len(filePaths) == 0 {
		http.Error(w, "没有有效的文件", 400)
		return
	}

	// 调用bt-shieldml检测整个目录
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
			Filename: originalName,
			Size:     fileInfo.size,
			MD5:      fileInfo.md5,
			SHA256:   fileInfo.sha256,
			Type:     fileInfo.ftype,
			Risk:     risk,
			Icon:     icon,
			Desc:     desc,
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
