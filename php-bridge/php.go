// php-bridge/php.go
package php_bridge

/*
#cgo CFLAGS: -Wall -Wextra -Werror -Wno-unused-parameter -I${SRCDIR}/include/php -I${SRCDIR}/include/php/Zend -I${SRCDIR}/include/php/TSRM -I${SRCDIR}/include/php/main
#cgo LDFLAGS: ${SRCDIR}/lib/libphp7.a -lm

#include <stdint.h>
#include <stdio.h>

int init(intptr_t fd_in, intptr_t fd_out);
int execute(void);
// void php_embed_shutdown(void); // 如果 C 代码提供了关闭函数
*/
import "C"
import (
	"bt-shieldml/pkg/logging"
	"fmt"
	"os"
	"sync"
	"time" // 用于 StopBridge 超时
)

// 全局变量存储持久化实例的句柄和状态
var (
	goStdinWriter    *os.File   // Go -> PHP
	goStdoutReader   *os.File   // PHP -> Go
	phpProcessExited chan error // PHP 进程退出信号
	initOnce         sync.Once  // 保证 C.init 和 C.execute goroutine 只运行一次
	startErr         error      // 存储初始化期间的错误
	stopOnce         sync.Once  // 保证清理只运行一次
	stopErr          error      // 存储停止时的错误
)

// StartBridge 获取持久化的 PHP 桥接实例句柄。如果尚未初始化，则进行初始化。
func StartBridge() (stdin *os.File, stdout *os.File, exited chan error, err error) {
	initOnce.Do(func() {
		// logging.InfoLogger.Println("Initializing persistent PHP bridge C layer (first call)...")

		var cStdinReader, goWriteStdinTmp *os.File
		var goReadStdoutTmp, cStdoutWriter *os.File
		var pipeErr error

		// 创建 Go -> PHP 管道
		cStdinReader, goWriteStdinTmp, pipeErr = os.Pipe()
		if pipeErr != nil {
			startErr = fmt.Errorf("failed to create stdin pipe: %w", pipeErr)
			return
		}

		// 创建 PHP -> Go 管道
		goReadStdoutTmp, cStdoutWriter, pipeErr = os.Pipe()
		if pipeErr != nil {
			cStdinReader.Close()
			goWriteStdinTmp.Close()
			startErr = fmt.Errorf("failed to create stdout pipe: %w", pipeErr)
			return
		}

		// 存储全局句柄
		goStdinWriter = goWriteStdinTmp
		goStdoutReader = goReadStdoutTmp
		phpProcessExited = make(chan error, 1) // Buffered channel

		// 传递文件描述符给 C 层
		cStdinFd := C.intptr_t(cStdinReader.Fd())
		cStdoutFd := C.intptr_t(cStdoutWriter.Fd())

		// 调用 C 初始化
		ret := C.init(cStdinFd, cStdoutFd)
		if ret != 0 {
			goStdinWriter.Close() // 清理 Go 这边的 pipe
			goStdoutReader.Close()
			cStdinReader.Close() // C 这边的也需要关闭
			cStdoutWriter.Close()
			startErr = fmt.Errorf("php bridge C initialization failed with code %d", ret)
			goStdinWriter = nil // 清理全局变量
			goStdoutReader = nil
			phpProcessExited = nil
			return
		}

		// 启动 goroutine 运行 C.execute 并监控
		go func(cr, cw *os.File) {
			defer func() {
				// logging.InfoLogger.Println("Closing C-side pipes...")
				cr.Close()
				cw.Close()
				// 可以在这里显式调用 PHP 关闭函数
				// C.php_embed_shutdown()
			}()
			exitCode := C.execute()
			// logging.InfoLogger.Printf("PHP bridge C.execute() finished with exit code: %d", exitCode)
			if exitCode != 0 {
				phpProcessExited <- fmt.Errorf("php bridge C execution failed with code %d", exitCode)
			} else {
				// 即使正常退出码为0，对于持久化模型来说，execute的退出也意味着桥接失效
				phpProcessExited <- fmt.Errorf("php bridge C execute returned unexpectedly (code 0)")
			}
			close(phpProcessExited)
		}(cStdinReader, cStdoutWriter) // 将 C 端管道传入

		// logging.InfoLogger.Println("Persistent PHP Bridge C layer started successfully.")
	})

	// 返回存储的句柄或错误
	if startErr != nil {
		return nil, nil, nil, startErr
	}
	// 再次检查全局变量，以防万一 initOnce 内部有异常跳出
	if goStdinWriter == nil || goStdoutReader == nil || phpProcessExited == nil {
		return nil, nil, nil, fmt.Errorf("bridge state inconsistent after initialization attempt")
	}

	return goStdinWriter, goStdoutReader, phpProcessExited, nil
}

// StopBridge 清理持久化的 PHP 桥接资源
func StopBridge() error {
	stopOnce.Do(func() {
		// logging.InfoLogger.Println("Stopping persistent PHP Bridge...")

		// 1. 关闭 Go 端的写入，向 PHP 发送 EOF 信号
		if goStdinWriter != nil {
			// logging.InfoLogger.Println("Closing Go stdin writer...")
			goStdinWriter.Close()
			goStdinWriter = nil // 防止重复关闭
		}

		// 2. 等待 PHP 进程退出 goroutine 发送信号 (带超时)
		if phpProcessExited != nil {
			// logging.InfoLogger.Println("Waiting for PHP bridge process to signal exit...")
			select {
			case err, ok := <-phpProcessExited:
				if ok && err != nil { // 通道未关闭且收到错误
					logging.ErrorLogger.Printf("PHP Bridge exited with error during StopBridge wait: %v", err)
					stopErr = err // 存储错误
				} else if ok { // 收到 nil (不应该发生，因为我们期待错误或关闭)
					logging.WarnLogger.Println("PHP Bridge process signaled normal exit (unexpected for persistent model) during StopBridge wait.")
				} else { // 通道已关闭
					logging.InfoLogger.Println("PHP Bridge exit channel was already closed.")
				}
			case <-time.After(5 * time.Second): // 5秒超时
				logging.ErrorLogger.Println("Timeout waiting for PHP bridge process to exit.")
				stopErr = fmt.Errorf("timeout waiting for bridge exit signal")
			}
			phpProcessExited = nil // 重置 channel 变量
		} else {
			logging.WarnLogger.Println("PHP Bridge exit channel was nil during stop.")
		}

		// 3. 关闭 Go 端的读取
		if goStdoutReader != nil {
			goStdoutReader.Close()
			goStdoutReader = nil // 防止重复关闭
		}

		// 理论上 C.php_embed_shutdown() 应该在这里调用（如果存在）

	})
	return stopErr // 返回存储的停止错误
}
