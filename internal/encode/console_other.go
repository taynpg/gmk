//go:build !windows

package encode

import (
	"io"
	"os"
)

// StdoutWriter 在类 Unix 平台直接返回 stdout（默认 UTF-8）。
func StdoutWriter() io.Writer {
	return os.Stdout
}
