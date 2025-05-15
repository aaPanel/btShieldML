// internal/ast/manager.go
package ast

import (
	phpbridge "bt-shieldml/php-bridge" // 确认包路径
	"bt-shieldml/pkg/logging"
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ASTManager 定义了接口
type ASTManager interface {
	GetAST(source []byte) (interface{}, error) // 返回解析后的 AST 结构
	GetWordsAndCallable(astRoot interface{}) ([]string, bool, error)
	GetOpSerial(astRoot interface{}) ([][]int, error)
	Cleanup() error
}

// PhpAstManager 管理与持久化 PHP AST 解析器进程的通信
type PhpAstManager struct {
	phpStdin  io.WriteCloser // Go -> PHP
	phpStdout io.ReadCloser  // PHP -> Go
	phpExited chan error     // 监控进程退出
	mu        sync.Mutex     // 保护对 PHP 进程管道的并发访问
	isActive  bool           // 标记桥接是否仍被认为可用
}

// NewPhpAstManager 创建管理器实例并初始化（或获取）持久化桥接
func NewPhpAstManager() (*PhpAstManager, error) {
	// 尝试启动或获取持久化桥接
	stdin, stdout, exited, startErr := phpbridge.StartBridge()
	if startErr != nil {
		// 如果启动失败，manager 无法工作
		logging.ErrorLogger.Printf("Failed to start or get persistent PHP bridge: %v", startErr)
		return nil, startErr
	}

	manager := &PhpAstManager{
		phpStdin:  stdin,
		phpStdout: stdout,
		phpExited: exited,
		isActive:  true,
	}

	// 启动后台监控协程
	go manager.monitorExit()

	return manager, nil
}

// monitorExit 监控持久化 PHP 进程的退出事件
func (m *PhpAstManager) monitorExit() {
	if m.phpExited == nil {
		logging.ErrorLogger.Println("AST Manager monitorExit: phpExited channel is nil.")
		return
	}
	// 等待退出信号
	err := <-m.phpExited

	// 加锁修改状态
	m.mu.Lock()
	defer m.mu.Unlock()

	// 只有在还是 active 状态时才标记为 inactive 并记录日志
	// 防止 Cleanup 先执行了
	if m.isActive {
		m.isActive = false // 标记桥接失效
		if err != nil {
			// 不输出日志
			// logging.ErrorLogger.Printf("Persistent PHP bridge process exited UNEXPECTEDLY: %v", err)
		} else {
			// 对于持久化模型，即使正常退出码也是意外的
			logging.ErrorLogger.Println("Persistent PHP bridge process exited UNEXPECTEDLY (returned normally).")
		}
		// 不需要在这里关闭管道，StopBridge 会处理
	}
}

// GetAST 发送源码到持久化桥接并获取解析后的 AST 结构
func (m *PhpAstManager) GetAST(source []byte) (interface{}, error) {
	m.mu.Lock()         // 在开始任何操作前获取锁
	defer m.mu.Unlock() // 保证函数返回时释放锁

	if !m.isActive || m.phpStdin == nil || m.phpStdout == nil {
		logging.ErrorLogger.Println("GetAST failed: PHP bridge is not active or pipes are nil.")
		return nil, fmt.Errorf("php bridge is not active or initialized")
	}

	// 在持有锁的情况下获取管道引用
	currentStdin := m.phpStdin
	currentStdout := m.phpStdout

	// 使用 context 控制超时，建议将 timeout 值设为可配置
	timeout := 60 * time.Second // 暂时增加到 60 秒，后续可配置
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // 确保 context 相关资源被清理

	resultChan := make(chan []byte, 1)
	errChan := make(chan error, 1)

	// 启动通信 goroutine，但我们在持有锁的情况下等待它完成
	go func() {
		astData, err := m.communicateWithBridge(currentStdin, currentStdout, source)
		if err != nil {
			// 先检查是否是因为 context 超时/取消导致的错误
			select {
			case <-ctx.Done():
				// 如果 context 已结束 (超时), 不再发送错误，因为超时会被主 select 处理
				return
			default:
				// 否则，发送通信错误
				errChan <- err
			}
		} else {
			// 发送结果前也检查 context
			select {
			case <-ctx.Done():
				// 如果 context 已结束 (超时), 不再发送结果
				return
			default:
				resultChan <- astData
			}
		}
	}()

	// 在持有锁的情况下等待通信结果、错误或超时
	select {
	case rawAstData := <-resultChan:
		parsedAst, parseErr := ParseAST(rawAstData)
		if parseErr != nil {
			logging.ErrorLogger.Printf("解析接收到的 AST 数据失败: %v", parseErr)
			return nil, fmt.Errorf("解析 AST 数据失败: %w", parseErr)
		}
		return parsedAst, nil // 返回解析后的结构
	case err := <-errChan:
		// 再次检查 context，防止错误与超时竞争
		select {
		case <-ctx.Done():
			logging.ErrorLogger.Printf("Timeout (%s) occurred, received error afterwards: %v", timeout, err)
			return nil, fmt.Errorf("timeout waiting for PHP bridge response") // 统一返回超时错误
		default:
			logging.ErrorLogger.Printf("Communication error with PHP bridge: %v", err)
			// 此时桥接可能已损坏，monitorExit 应该会检测到进程退出
			return nil, fmt.Errorf("php bridge communication failed: %w", err)
		}
	case <-ctx.Done():
		logging.ErrorLogger.Printf("Timeout (%s) waiting for PHP bridge response.", timeout)
		return nil, fmt.Errorf("timeout waiting for PHP bridge response")
	}
}

// communicateWithBridge 处理底层发送/接收逻辑 (函数保持不变)
func (m *PhpAstManager) communicateWithBridge(stdin io.Writer, stdout io.Reader, source []byte) ([]byte, error) {
	srcLen := len(source)
	if srcLen == 0 {
		return nil, fmt.Errorf("cannot process empty source code")
	}
	// 1. 发送长度头
	lenStr := strconv.Itoa(srcLen) + "\n"
	if _, err := stdin.Write([]byte(lenStr)); err != nil {
		return nil, fmt.Errorf("failed to write length to php bridge: %w", err)
	}
	// 2. 发送源代码
	if _, err := stdin.Write(source); err != nil {
		return nil, fmt.Errorf("failed to write source to php bridge: %w", err)
	}
	// 3. 读取响应长度头
	reader := bufio.NewReader(stdout)
	lenBytes, err := reader.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("failed to read length from php bridge (EOF reached), bridge likely closed unexpectedly")
		}
		return nil, fmt.Errorf("failed to read length from php bridge: %w", err)
	}
	resultLenStr := strings.TrimSpace(string(lenBytes))
	resultLen, err := strconv.Atoi(resultLenStr)
	if err != nil {
		errorLine, _ := reader.ReadString('\n')
		return nil, fmt.Errorf("failed to parse result length '%s' from php bridge: %w. PHP output: %s", resultLenStr, err, strings.TrimSpace(errorLine))
	}
	if resultLen < 0 {
		errorLine, _ := reader.ReadString('\n')
		return nil, fmt.Errorf("php bridge returned invalid negative length %d. PHP output: %s", resultLen, strings.TrimSpace(errorLine))
	}
	if resultLen == 0 {
		errorLine, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			logging.WarnLogger.Printf("Could not read error details after zero length: %v", readErr)
		}
		return nil, fmt.Errorf("php bridge reported a parse error (length 0). PHP error: %s", strings.TrimSpace(errorLine))
	}
	// 4. 读取 AST 数据
	astData := make([]byte, resultLen)
	bytesRead, err := io.ReadFull(reader, astData)
	if err != nil {
		return nil, fmt.Errorf("failed to read full AST data from php bridge (expected %d, got %d): %w", resultLen, bytesRead, err)
	}
	return astData, nil
}

// GetWordsAndCallable 从解析后的 AST 中提取词汇和可调用状态
// 参考 cloudwalker Ast.go 的 GetWordsAndCallable
func (m *PhpAstManager) GetWordsAndCallable(astRoot interface{}) ([]string, bool, error) {
	if astRoot == nil {
		return nil, false, fmt.Errorf("cannot process nil AST")
	}

	var words []string
	callable := false

	// 内部遍历函数
	var checkNameAndKind func(node interface{}, nameChecker func(string) bool, kindChecker func(int) bool)
	checkNameAndKind = func(node interface{}, nameChecker func(string) bool, kindChecker func(int) bool) {
		if node == nil {
			return
		}

		switch value := node.(type) {
		case astNode: // 使用本地定义的 astNode 结构
			// 检查 kind
			if kindChecker(value.Kind) {
				// 特殊处理可能包含名称的节点（例如 AST_VAR, AST_CONST 等）
				if nameMap, ok := value.Children.(map[string]interface{}); ok {
					if nameVal, exists := nameMap["name"]; exists {
						if nameStr, ok := nameVal.(string); ok {
							if nameChecker(nameStr) {
								// 如果名称检查器返回 true (例如, 添加到列表)

							}
						}
					}
				}
			}
			// 递归子节点
			checkNameAndKind(value.Children, nameChecker, kindChecker)
		case []interface{}:
			for _, item := range value {
				checkNameAndKind(item, nameChecker, kindChecker)
			}
		case map[string]interface{}:
			// 如果 map 本身是 astNode 结构（由 transformAstNode 处理）
			if isAstNodeMap(value) {
				transformed := transformAstNode(value) // 确保转换
				checkNameAndKind(transformed, nameChecker, kindChecker)
			} else {
				// 否则，遍历 map 的值
				keys := make([]string, 0, len(value))
				for k := range value {
					keys = append(keys, k)
				}
				sort.Strings(keys) // 保持顺序一致性
				for _, k := range keys {
					checkNameAndKind(value[k], nameChecker, kindChecker)
				}
			}
			// No default needed as other types (string, float64, bool, nil) are handled implicitly by not recursing
		}
	}

	// 执行遍历
	checkNameAndKind(astRoot,
		func(s string) bool { // nameChecker
			words = append(words, s)
			return true
		},
		func(k int) bool { // kindChecker
			// 269: AST_INCLUDE_OR_EVAL
			// 265: AST_SHELL_EXEC
			// 515: AST_CALL
			// 768: AST_METHOD_CALL
			// 769: AST_STATIC_CALL
			if k == 269 || k == 265 || k == 515 || k == 768 || k == 769 {
				callable = true
			}
			return true // 继续遍历
		},
	)

	return words, callable, nil
}

/**
 * @Description: 从解析后的 AST 中提取操作序列
 * @author: Mr wpl
 * @param astRoot interface{}: AST根节点
 * @return [][]int 操作序列
 * @return [][]int: 清洗后的操作序列集合（每个子数组表示一个操作链）
 */
func (m *PhpAstManager) GetOpSerial(astRoot interface{}) ([][]int, error) {
	if astRoot == nil {
		return nil, fmt.Errorf("cannot process nil AST")
	}

	var result [][]int
	var currentSerial []int
	queue := []opQueueNode{} // 使用本地定义的 opQueueNode

	// 将根节点添加到队列
	queue = append(queue, opQueueNode{Key: "root", Value: astRoot, Layer: 0, Father: nil})

	// BFS 遍历
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		switch value := node.Value.(type) {
		case astNode: // 使用本地定义的 astNode 结构
			currentSerial = append(currentSerial, value.Kind)
			// 将子节点加入队列
			if value.Children != nil {
				queue = append(queue, opQueueNode{Key: "children", Value: value.Children, Layer: node.Layer + 1, Father: &value})
			}
		case string, float64, bool:
			// 基本类型，在序列中通常不表示，忽略
		case nil:
			// 在 Cloudwalker 中用于分隔块，这里当遇到 map 或 array 的子节点处理完后添加
			if node.Key == "separator" && len(currentSerial) > 0 {
				finishSerial := make([]int, 0, len(currentSerial)+1)
				// Cloudwalker 添加了父节点 Kind，这里也遵循
				if node.Father != nil {
					finishSerial = append(finishSerial, node.Father.Kind)
				} else {
					// 如果根节点直接是数组/map，可能没有父 Kind
					// finishSerial = append(finishSerial, -1) // 或者一个特殊标记
				}
				finishSerial = append(finishSerial, currentSerial...)
				result = append(result, finishSerial)
				currentSerial = []int{} // 开始新序列
			}
		case []interface{}:
			for i, item := range value {
				queue = append(queue, opQueueNode{Key: strconv.Itoa(i), Value: item, Layer: node.Layer + 1, Father: node.Father})
			}
			// 数组处理完毕后，添加分隔符 (如果它是由 'children' 键产生的)
			if node.Key == "children" && len(value) > 0 {
				queue = append(queue, opQueueNode{Key: "separator", Value: nil, Layer: node.Layer + 1, Father: node.Father})
			}
		case map[string]interface{}:
			// 检查是否是 astNode 的 map 形式
			if isAstNodeMap(value) {
				// 应该已经被 transformAstNode 处理了，但以防万一
				transformed := transformAstNode(value)
				queue = append(queue, opQueueNode{Key: node.Key, Value: transformed, Layer: node.Layer, Father: node.Father}) // Re-queue transformed node
			} else {
				// 普通 map，按 key 排序遍历
				keys := make([]string, 0, len(value))
				for k := range value {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					queue = append(queue, opQueueNode{Key: k, Value: value[k], Layer: node.Layer + 1, Father: node.Father})
				}
				// map 处理完毕后，添加分隔符 (如果它是由 'children' 键产生的)
				if node.Key == "children" && len(value) > 0 {
					queue = append(queue, opQueueNode{Key: "separator", Value: nil, Layer: node.Layer + 1, Father: node.Father})
				}
			}
			// default: // 其他未处理类型
			// logging.WarnLogger.Printf("GetOpSerial encountered unhandled type: %T for key %s", value, node.Key)
		}
	}

	// 清理可能遗留的 currentSerial
	if len(currentSerial) > 0 {
		finishSerial := make([]int, 0, len(currentSerial)+1)
		// 尝试获取最后一个节点的父节点 Kind
		// Note: 这可能不完全准确，取决于 BFS 的结束状态
		// if len(result) > 0 && len(result[len(result)-1]) > 0 {
		//     finishSerial = append(finishSerial, result[len(result)-1][0]) // Use previous father kind? Risky.
		// }
		finishSerial = append(finishSerial, currentSerial...)
		result = append(result, finishSerial)
	}

	// 清理重复序列
	maxCleanTimes := 10
	maxCleanLength := 5
	cleanedResult := cleanOpSerial(result, maxCleanLength) // Initial clean
	for i := 1; i < maxCleanTimes && len(result) != len(cleanedResult); i++ {
		result = cleanedResult
		cleanedResult = cleanOpSerial(result, maxCleanLength)
	}

	return cleanedResult, nil
}

// opQueueNode 用于 BFS 遍历 AST 以提取操作序列
type opQueueNode struct {
	Key    string
	Value  interface{}
	Layer  int
	Father *astNode // 指向父 astNode 结构 (如果可用)
}

// isAstNodeMap 辅助函数，检查 map 是否具有 astNode 的特征
func isAstNodeMap(data map[string]interface{}) bool {
	_, kindOk := data["kind"]
	return kindOk
}

/**
 * @Description: 清理重复序列
 * @author: Mr wpl
 * @param data [][]int: 操作序列
 * @param maxLen int: 最大长度
 * @return [][]int: 清洗后的操作序列集合
 */
func cleanOpSerial(data [][]int, maxLen int) [][]int {
	blockCompare := func(data []int, i int, length int) bool {
		if i+2*length > len(data) {
			return false
		}
		for k := 0; k < length; k++ {
			if data[i+k] != data[i+length+k] {
				return false
			}
		}
		return true
	}

	var result [][]int
	for _, v := range data {
		tmp := make([]int, len(v))
		copy(tmp, v) // 操作副本
		for length := maxLen; length >= 1; length-- {
			j := 0
			for j < len(tmp) { // 注意循环条件变化
				if blockCompare(tmp, j, length) {
					// 删除重复块：tmp = append(tmp[:j], tmp[j+length:]...)
					// 创建新切片以避免潜在的内存问题
					newTmp := make([]int, 0, len(tmp)-length)
					newTmp = append(newTmp, tmp[:j]...)
					newTmp = append(newTmp, tmp[j+length:]...)
					tmp = newTmp
					continue
				}
				j++
			}
		}
		result = append(result, tmp)
	}
	return result
}

/**
 * @Description: 清理持久化的 PHP 桥接进程
 * @author: Mr wpl
 * @return error 错误
 */
func (m *PhpAstManager) Cleanup() error {
	// 调用 php-bridge 的 StopBridge 来处理清理
	// StopBridge 内部使用了 sync.Once 保证只清理一次
	err := phpbridge.StopBridge() // 这里会处理 stdin/stdout 的关闭
	m.mu.Lock()
	m.isActive = false // 确保标记为 inactive
	m.mu.Unlock()
	return err
}
