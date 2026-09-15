package encode

import (
	"bytes"
	"testing"
	"unicode/utf8"
)

func TestEnsureUTF8(t *testing.T) {
	// 合法 UTF-8 原样返回
	utf8Bytes := []byte("hello 中文")
	if got := EnsureUTF8(utf8Bytes); !bytes.Equal(got, utf8Bytes) {
		t.Fatalf("合法 UTF-8 被意外转换: %q", got)
	}

	// GBK 编码的 "中文" 应被转成 UTF-8
	gbkBytes, err := gbkEncoder.Bytes([]byte("中文"))
	if err != nil {
		t.Fatalf("构造 GBK 测试数据失败: %v", err)
	}
	if utf8.Valid(gbkBytes) {
		t.Fatal("测试前置条件不成立：GBK 字节恰好是合法 UTF-8")
	}
	got := EnsureUTF8(gbkBytes)
	if string(got) != "中文" {
		t.Fatalf("GBK->UTF-8 结果错误: %q", got)
	}
}

func TestLineWriter(t *testing.T) {
	gbkLine, _ := gbkEncoder.Bytes([]byte("编译错误信息"))

	var buf bytes.Buffer
	lw := NewLineWriter(&buf)

	// 模拟多字节字符被切成两半、跨多次 Write 到达
	cut := len(gbkLine) / 2
	if _, err := lw.Write(gbkLine[:cut]); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("行未结束不应有输出，得到 %q", buf.Bytes())
	}
	if _, err := lw.Write(gbkLine[cut:]); err != nil {
		t.Fatal(err)
	}
	if _, err := lw.Write([]byte("\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := lw.Flush(); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); got != "编译错误信息\r\n" {
		t.Fatalf("跨行拼接转码结果错误: %q", got)
	}
}
