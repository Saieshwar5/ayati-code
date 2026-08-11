package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
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
