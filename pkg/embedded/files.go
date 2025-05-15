/*
 * @Date: 2025-05-13 16:54:58
 * @Editors: Mr wpl
 * @Description:
 */
// pkg/embedded/files.go
package embedded

import (
	"embed"
	"io/fs"
)

//go:embed config.yaml
//go:embed data/models/ProcessSVM.model.info
//go:embed data/models/ProcessSVM.model.model
//go:embed data/models/Words.model
//go:embed data/signatures/Webshells_rules.yar
var EmbeddedFiles embed.FS

/**
 * @Description: 获取嵌入文件的内容
 * @author: Mr wpl
 * @param path string: 文件路径
 * @return []byte: 文件内容
 * @return error: 错误
 */
func GetFileContent(path string) ([]byte, error) {
	return EmbeddedFiles.ReadFile(path)
}

/**
 * @Description: 获取嵌入文件系统
 * @author: Mr wpl
 * @return fs.FS: 文件系统
 */
func GetFS() fs.FS {
	return EmbeddedFiles
}
