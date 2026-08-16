package shell

import (
	"strings"
	"testing"
)

func TestBoundedBufferKeepsHeadAndTail(t *testing.T) {
	buffer := newBoundedBuffer(10)
	input := "abcdefghijklmnopqrst"
	if _, err := buffer.Write([]byte(input[:7])); err != nil {
		t.Fatal(err)
	}
	if _, err := buffer.Write([]byte(input[7:])); err != nil {
		t.Fatal(err)
	}
	got := buffer.String()
	if !strings.HasPrefix(got, "abcde") || !strings.HasSuffix(got, "pqrst") {
		t.Fatalf("bounded output = %q", got)
	}
	if !strings.Contains(got, "10 bytes omitted") || !buffer.Truncated() || buffer.Total() != 20 {
		t.Fatalf("buffer metadata: output=%q truncated=%v total=%d", got, buffer.Truncated(), buffer.Total())
	}
}

func TestBoundedBufferReturnsExactSmallOutput(t *testing.T) {
	buffer := newBoundedBuffer(10)
	_, _ = buffer.Write([]byte("hello"))
	if got := buffer.String(); got != "hello" {
		t.Fatalf("output = %q", got)
	}
	if buffer.Truncated() {
		t.Fatal("small output marked truncated")
	}
}
