package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestVersionNeedsNoConfiguration(t *testing.T) {
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	code := Run(context.Background(), []string{"--version"}, strings.NewReader(""), &output, &errorOutput)
	if code != 0 || strings.TrimSpace(output.String()) != "ayati dev" {
		t.Fatalf("code = %d, output = %q, error = %q", code, output.String(), errorOutput.String())
	}
}

func TestRunRequiresFireworksKey(t *testing.T) {
	t.Setenv("FIREWORKS_API_KEY", "")
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	code := Run(context.Background(), []string{"--model", "test"}, strings.NewReader(""), &output, &errorOutput)
	if code != 2 || !strings.Contains(errorOutput.String(), "FIREWORKS_API_KEY") {
		t.Fatalf("code = %d, error = %q", code, errorOutput.String())
	}
}
