// internal/ast/parser.go
package ast

import (
	"bt-shieldml/pkg/logging"
	"encoding/json"
	"fmt"
	"reflect"
)

// ParseAST 解析 JSON 并将其转换为 astNode 结构
func ParseAST(data []byte) (interface{}, error) {
	var rawData interface{} // 先解析到通用 interface{}

	if err := json.Unmarshal(data, &rawData); err != nil {
		// 检查是否是有效的 JSON，但 Unmarshal 内部会做
		logging.ErrorLogger.Printf("Failed to unmarshal AST JSON data (first 100 bytes): %s. Error: %v", string(data[:min(100, len(data))]), err)
		return nil, fmt.Errorf("invalid JSON received from PHP bridge: %w", err)
	}

	// 检查顶层结构是否是 map，并且包含 "ast" 键 (类似 cloudwalker)
	if rootMap, ok := rawData.(map[string]interface{}); ok {
		if astValue, exists := rootMap["ast"]; exists {
			transformedAst := transformAstNode(astValue)
			// logging.InfoLogger.Println("AST transformation complete.")
			return transformedAst, nil
		} else if reason, exists := rootMap["reason"]; exists {
			// 处理 PHP 解析器直接返回的错误
			if reasonStr, ok := reason.(string); ok {
				logging.ErrorLogger.Printf("PHP parser returned error: %s", reasonStr)
				return nil, fmt.Errorf("php parser error: %s", reasonStr)
			}
			logging.ErrorLogger.Printf("PHP parser returned unknown error structure: %v", reason)
			return nil, fmt.Errorf("php parser returned unknown error")
		} else {
			// 顶层是 map 但没有 "ast" 或 "reason"
			logging.WarnLogger.Printf("Received JSON map, but missing 'ast' or 'reason' key. Raw data: %v", rootMap)
			// 异常捕获
			return nil, fmt.Errorf("unexpected JSON structure: missing 'ast' or 'reason' key in root map")
		}
	}

	// 如果顶层不是 map，尝试直接转换 (可能是数组或单个节点?)
	logging.WarnLogger.Printf("Received JSON data is not a map with 'ast' key, attempting direct transformation. Type: %T", rawData)
	transformedAst := transformAstNode(rawData)
	return transformedAst, nil
}

// transformAstNode 将interface{} 转换为 astNode 结构
func transformAstNode(nodeData interface{}) interface{} {
	if nodeData == nil {
		return nil
	}

	switch value := nodeData.(type) {
	case float64:
		// JSON 数字默认为 float64，我们假设 kind, flags, lineno 都是整数
		// 但这里可能是其他数值，直接返回
		return value
	case string:
		return value
	case bool:
		return value
	case []interface{}:
		// 如果是数组，递归转换数组中的每个元素
		resultArray := make([]interface{}, len(value))
		for i, v := range value {
			resultArray[i] = transformAstNode(v)
		}
		return resultArray
	case map[string]interface{}:
		// 如果是 map，检查它是否代表一个 astNode
		if kindVal, kOk := value["kind"]; kOk {
			// 尝试将 kind, flags, lineno 转换为 int
			kind, kindIntOk := getInt(kindVal)
			flags, _ := getInt(value["flags"])   // 可能不存在，getInt 会处理
			lineno, _ := getInt(value["lineno"]) // 可能不存在，getInt 会处理

			if kindIntOk { // 只要 kind 是整数，就认为是 astNode
				return astNode{
					Kind:     kind,
					Flag:     flags, // flags 和 lineno 失败时为 0
					LineNo:   lineno,
					Children: transformAstNode(value["children"]), // 递归转换 children
				}
			}
		}
		// 如果不是 astNode 结构，就递归转换 map 中的值
		resultMap := make(map[string]interface{})
		for k, v := range value {
			resultMap[k] = transformAstNode(v)
		}
		return resultMap
	default:
		logging.WarnLogger.Printf("Unhandled type in transformAstNode: %T", value)
		return value
	}
}

// getInt 尝试从 interface{} (通常是 float64) 获取 int 值
func getInt(v interface{}) (int, bool) {
	if v == nil {
		return 0, false // 不存在或为 nil
	}
	// 常见情况：JSON 数字解析为 float64
	if fVal, ok := v.(float64); ok {
		return int(fVal), true
	}
	// 其他整数类型（以防万一）
	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(val.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		// 注意 uint 到 int 的转换可能溢出，但对于 AST 节点类型通常没问题
		return int(val.Uint()), true
	}
	return 0, false // 无法转换为 int
}

// min 函数 (用于日志截断)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
