//go:build windows

package encode

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// 注意：不能通过 SetConsoleOutputCP(65001) 全局切换控制台代码页——
// 子进程（cmd.exe / cl.exe 等）会继承该设置，把本应是 GBK 的批处理/输出
// 按 UTF-8 解释，字节在到达我们的转码器之前就被破坏。
// 正确做法：保持系统代码页不变，由本进程在写入控制台时走 WriteConsoleW(UTF-16)，
// 该 API 与控制台代码页无关；重定向到管道/文件时直接写 UTF-8 字节。
//
// 颜色通过 ANSI 转义序列实现，需要在控制台输出缓冲区开启
// ENABLE_VIRTUAL_TERMINAL_PROCESSING，否则 cmd.exe 会把 ESC 序列当普通字符显示。

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
	procGetStdHandle   = kernel32.NewProc("GetStdHandle")
	procWriteConsoleW  = kernel32.NewProc("WriteConsoleW")
)

const (
	stdOutputHandle                 = uint32(-11 & 0xFFFFFFFF) // STD_OUTPUT_HANDLE
	enableVirtualTerminalProcessing = 0x0004
)

// EnableVirtualTerminalProcessing 为标准输出控制台开启 ANSI 转义序列支持，
// 使得日志的颜色代码能被 cmd.exe 正确渲染。该调用只影响转义序列的解释，
// 不改变代码页，因此不会破坏子进程的 GBK 输出。失败时静默忽略（旧版系统不支持）。
func EnableVirtualTerminalProcessing() {
	h, _, _ := procGetStdHandle.Call(uintptr(stdOutputHandle))
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return
	}
	var mode uint32
	if r, _, _ := procGetConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode))); r == 0 {
		return // 不是控制台
	}
	// 只追加 VT 标志，不改动其他位，避免影响控制台既有行为
	procSetConsoleMode.Call(h, uintptr(mode|enableVirtualTerminalProcessing))
}

// winConsoleWriter 把 UTF-8 文本以 UTF-16 经 WriteConsoleW 写入控制台。
type winConsoleWriter struct {
	handle syscall.Handle
}

func (w *winConsoleWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	u16, err := syscall.UTF16FromString(string(p))
	if err != nil {
		return 0, err
	}
	if len(u16) > 0 {
		u16 = u16[:len(u16)-1] // 去掉结尾 NUL
	}
	if len(u16) == 0 {
		return len(p), nil
	}

	// WriteConsoleW 理论上一次写完，仍循环处理部分写入
	for len(u16) > 0 {
		var written uint32
		r1, _, _ := procWriteConsoleW.Call(
			uintptr(w.handle),
			uintptr(unsafe.Pointer(&u16[0])),
			uintptr(len(u16)),
			uintptr(unsafe.Pointer(&written)),
			0,
		)
		if r1 == 0 {
			// 写入失败（句柄不再是控制台等），返回错误让上层处理；
			// 不再回退到 os.Stdout.Write，避免与 Go 运行时的控制台 I/O 冲突。
			return 0, syscall.EINVAL
		}
		if written == 0 {
			break
		}
		u16 = u16[written:]
	}
	return len(p), nil
}

// StdoutWriter 返回标准输出对应的 Writer：
// 真实控制台返回 WriteConsoleW 写入器，管道/文件重定向返回原始文件（写 UTF-8 字节）。
func StdoutWriter() io.Writer {
	handle := syscall.Handle(os.Stdout.Fd())
	var mode uint32
	r1, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 != 0 {
		return &winConsoleWriter{handle: handle}
	}
	return os.Stdout
}
