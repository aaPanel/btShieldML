package config

import (
	"bt-shieldml/pkg/embedded"
	"bt-shieldml/pkg/logging"
	"bt-shieldml/pkg/types"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

/**
 * @Description: 加载配置文件，优先使用嵌入文件
 * @author: Mr wpl
 * @param configPath string: 配置文件路径
 * @return *types.Config: 配置
 * @return error: 错误
 */
func LoadConfig(configPath string) (*types.Config, error) {
	var configData []byte
	var err error

	// 优先尝试从嵌入文件加载
	configData, err = embedded.GetFileContent("config.yaml")
	if err != nil {
		logging.InfoLogger.Printf("未找到嵌入配置文件，尝试从磁盘加载: %v", err)

		// 尝试从磁盘加载
		configData, err = os.ReadFile(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				logging.WarnLogger.Printf("配置文件 %s 不存在，使用默认配置", configPath)
				return GetDefaultConfig(), nil
			}
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	cfg := &types.Config{}
	if err := yaml.Unmarshal(configData, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证必要的配置
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

/**
 * @Description: 获取默认配置
 * @author: Mr wpl
 * @return *types.Config: 配置
 */
func GetDefaultConfig() *types.Config {
	return &types.Config{
		DataPaths: types.DataPaths{
			Models:     "data/models",
			Signatures: "data/signatures",
			Config:     "data/config",
		},
		Performance: types.Performance{
			Concurrency: 8,
		},
		Output: types.Output{
			Format: "console",
		},
		EnabledAnalyzers: []string{
			"regex",
			"yara",
			"statistical",
			"bayes_words",
			"svm_prosses",
		},
	}
}

/**
 * @Description: 验证配置
 * @author: Mr wpl
 * @param cfg *types.Config: 配置
 * @return error: 错误
 */
func validateConfig(cfg *types.Config) error {
	// 实现配置验证逻辑
	return nil
}

// Helper function to check if a command-line flag was explicitly set
// (Requires integrating with flag package in main.go)
func flagWasSet(name string) bool {
	return false
}
