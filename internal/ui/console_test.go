package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPromptStopsWhenContextIsCanceled(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	console := New(reader, &bytes.Buffer{}, &bytes.Buffer{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := console.Prompt(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Prompt error = %v", err)
	}
}

func TestToolCallShowsPurposeBeforeCommand(t *testing.T) {
	var output bytes.Buffer
	console := New(strings.NewReader(""), &output, &bytes.Buffer{})
	console.ToolCall("Verify the changed package", "go test ./internal/agent")
	want := "purpose> Verify the changed package\nshell> go test ./internal/agent\n"
	if output.String() != want {
		t.Fatalf("output = %q", output.String())
	}
}
