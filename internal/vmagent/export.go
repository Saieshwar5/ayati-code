package vmagent

import (
	"bytes"
	"context"
	"io"
)

// TarTree serializes root into a gzip tar buffer, skipping .git and symlinks.
// The controller passes the result to Client.Bootstrap so the microVM receives
// a working tree without Git or secret files.
func TarTree(root string) ([]byte, error) {
	var buffer bytes.Buffer
	writer := newTarGzipWriter(&buffer)
	if err := tarDirectory(context.Background(), writer, root); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// ExtractTree applies a gzip tar stream into root. .git entries and symlinks
// are ignored and traversal outside root is rejected.
func ExtractTree(source io.Reader, root string) error {
	reader, err := newTarGzipReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	return extractTar(context.Background(), reader, root)
}
