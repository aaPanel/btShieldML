/*
 * @Date: 2025-04-15 11:09:30
 * @Editors: Mr wpl
 * @Description:定义AST节点结构体
 */
package ast

// 该结构体定义了PHP AST中的一个节点，包含节点类型、标志、行号和子节点
type astNode struct {
	Kind     int         `json:"kind"`     // 对应 JSON 中的 kind
	Flag     int         `json:"flags"`    // 对应 JSON 中的 flags
	LineNo   int         `json:"lineno"`   // 对应 JSON 中的 lineno
	Children interface{} `json:"children"` // 子节点，类型不确定
}
