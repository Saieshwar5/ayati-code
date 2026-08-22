package vmagent

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type tarGzipReader struct {
	gzipReader *gzip.Reader
	tarReader  *tar.Reader
}

func newTarGzipReader(source io.Reader) (*tarGzipReader, error) {
	gzipReader, err := gzip.NewReader(source)
	if err != nil {
		return nil, err
	}
	return &tarGzipReader{gzipReader: gzipReader, tarReader: tar.NewReader(gzipReader)}, nil
}

func (t *tarGzipReader) Next() (*tar.Header, error) { return t.tarReader.Next() }

func (t *tarGzipReader) Read(buffer []byte) (int, error) { return t.tarReader.Read(buffer) }

func (t *tarGzipReader) Close() error { return t.gzipReader.Close() }

type tarGzipWriter struct {
	gzipWriter *gzip.Writer
	tarWriter  *tar.Writer
}

func newTarGzipWriter(target io.Writer) *tarGzipWriter {
	gzipWriter := gzip.NewWriter(target)
	return &tarGzipWriter{gzipWriter: gzipWriter, tarWriter: tar.NewWriter(gzipWriter)}
}

func (t *tarGzipWriter) WriteHeader(header *tar.Header) error { return t.tarWriter.WriteHeader(header) }

func (t *tarGzipWriter) Write(buffer []byte) (int, error) { return t.tarWriter.Write(buffer) }

func (t *tarGzipWriter) Close() error {
	if err := t.tarWriter.Close(); err != nil {
		return err
	}
	return t.gzipWriter.Close()
}

// extractTar copies the tar stream into root, skipping .git and symlinks.
func extractTar(_ context.Context, reader *tarGzipReader, root string) error {
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(header.Name)
		if name == "." || strings.HasPrefix(name, ".git") {
			continue
		}
		target := filepath.Join(root, name)
		if !strings.HasPrefix(target, filepath.Clean(root)+string(filepath.Separator)) && target != filepath.Clean(root) {
			return errPathOutsideRoot
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			// Skip symlinks, devices, and other special entries.
		}
	}
}

var errPathOutsideRoot = errPathTraversal

func tarDirectory(_ context.Context, writer *tarGzipWriter, root string) error {
	root = filepath.Clean(root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if strings.HasPrefix(relative, ".git") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
