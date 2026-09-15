// Package encode 处理 Windows 下子进程输出的编码问题：
// 若字节流不是合法 UTF-8，则按 GBK（用 GB18030 解码器，向下兼容 GBK/GB2312）转为 UTF-8。
package encode

import (
	"bytes"
	"io"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

var gbkDecoder = simplifiedchinese.GB18030.NewDecoder()
var gbkEncoder = simplifiedchinese.GB18030.NewEncoder()

// EnsureUTF8 对整块字节做判定：合法 UTF-8 原样返回，否则按 GBK 解码为 UTF-8。
// 适用于"先完整捕获、再统一处理"的场景（如 vswhere / vcvars 的 set 输出）。
func EnsureUTF8(b []byte) []byte {
	if len(b) == 0 || utf8.Valid(b) {
		return b
	}
	out, err := gbkDecoder.Bytes(b)
	if err != nil {
		return b
	}
	return out
}

// EnsureString 是 EnsureUTF8 的字符串版本。
func EnsureString(s string) string {
	return string(EnsureUTF8([]byte(s)))
}

// utf8BOM 是 UTF-8 BOM 字节序。
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// StripBOM 去掉开头的 UTF-8 BOM（记事本等编辑器常会带上）。
func StripBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, utf8BOM)
}

// UTF8ToGBK 将 UTF-8 编码为 GBK（GB18030），用于生成按 OEM 代码页解析的批处理文件。
// 纯 ASCII / 编码失败时原样返回。
func UTF8ToGBK(b []byte) []byte {
	if utf8.Valid(b) {
		if out, err := gbkEncoder.Bytes(b); err == nil {
			return out
		}
	}
	return b
}

// LineWriter 是面向字节流的实时转码 Writer：
// 子进程的输出可能在任意读取边界把多字节字符切成两半，因此这里按行
// （以 \n 或 \r 为界）缓冲，只对"完整片段"做 UTF-8 判定/GBK 解码，
// 避免跨边界误判；同时保证 ninja 之类用 \r 刷新进度条的场景仍然实时可见。
type LineWriter struct {
	w   io.Writer
	mu  sync.Mutex
	buf []byte
}

func NewLineWriter(w io.Writer) *LineWriter {
	return &LineWriter{w: w}
}

func (l *LineWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.buf = append(l.buf, p...)

	for {
		idx := delimiterIndex(l.buf)
		if idx < 0 {
			break
		}
		chunk := l.buf[:idx+1]
		l.buf = l.buf[idx+1:]
		if err := l.writeChunk(chunk); err != nil {
			l.mu.Unlock()
			return 0, err
		}
	}

	// 单行过长且长期没有换行符时兜底刷出，避免无限占用内存
	if len(l.buf) > 64*1024 {
		chunk := l.buf
		l.buf = nil
		if err := l.writeChunk(chunk); err != nil {
			l.mu.Unlock()
			return 0, err
		}
	}
	l.mu.Unlock()
	return len(p), nil
}

// Flush 在子进程结束后把残余字节刷出。
func (l *LineWriter) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.buf) == 0 {
		return nil
	}
	chunk := l.buf
	l.buf = nil
	return l.writeChunk(chunk)
}

func (l *LineWriter) writeChunk(chunk []byte) error {
	_, err := l.w.Write(EnsureUTF8(chunk))
	return err
}

// delimiterIndex 返回最早的 \r 或 \n 的下标，没有则返回 -1。
func delimiterIndex(b []byte) int {
	for i, c := range b {
		if c == '\n' || c == '\r' {
			return i
		}
	}
	return -1
}
