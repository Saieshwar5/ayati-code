package config

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

const maxInputBytes = 1 << 20

func Configure(ctx context.Context, input io.Reader, output, errorOutput io.Writer) int {
	path, err := DefaultPath()
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	values, loadErr := Load(path)
	exists := loadErr == nil
	if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		fmt.Fprintf(errorOutput, "perpetual: load configuration: %v\n", loadErr)
		return 1
	}
	fmt.Fprintf(output, "Perpetual configuration\nConfiguration: %s\n\n", path)
	reader := bufio.NewReader(input)
	keyLabel := "Fireworks API key: "
	modelLabel := "Fireworks model: "
	if exists {
		keyLabel = "Fireworks API key (leave blank to keep saved key): "
		modelLabel = fmt.Sprintf("Fireworks model [%s]: ", values.Model)
	}
	key, err := readSecret(ctx, reader, input, output, keyLabel)
	if err != nil {
		return configError(errorOutput, err)
	}
	model, err := readValue(ctx, reader, output, modelLabel)
	if err != nil {
		return configError(errorOutput, err)
	}
	if strings.TrimSpace(key) != "" {
		values.FireworksAPIKey = key
	}
	if strings.TrimSpace(model) != "" {
		values.Model = model
	}
	if err := Save(path, values); err != nil {
		return configError(errorOutput, err)
	}
	fmt.Fprintf(output, "Configuration saved to %s\n", path)
	return 0
}

func readSecret(
	ctx context.Context, reader *bufio.Reader, input io.Reader, output io.Writer, label string,
) (string, error) {
	fmt.Fprint(output, label)
	restore := func() error { return nil }
	if file, ok := input.(*os.File); ok {
		function, err := disableEcho(file)
		if err != nil && !errors.Is(err, syscall.ENOTTY) {
			fmt.Fprintln(output)
			return "", fmt.Errorf("hide secret input: %w", err)
		}
		if err == nil {
			restore = function
		}
	}
	value, err := readLine(ctx, reader)
	restoreErr := restore()
	fmt.Fprintln(output)
	if err == nil && restoreErr != nil {
		return "", fmt.Errorf("restore terminal input: %w", restoreErr)
	}
	return value, err
}

func readValue(ctx context.Context, reader *bufio.Reader, output io.Writer, label string) (string, error) {
	fmt.Fprint(output, label)
	return readLine(ctx, reader)
}

func readLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	type result struct {
		value string
		err   error
	}
	finished := make(chan result, 1)
	go func() {
		var value strings.Builder
		for {
			part, prefix, err := reader.ReadLine()
			if err != nil {
				finished <- result{err: err}
				return
			}
			if value.Len()+len(part) > maxInputBytes {
				finished <- result{err: errors.New("input exceeds 1 MiB")}
				return
			}
			value.Write(part)
			if !prefix {
				finished <- result{value: value.String()}
				return
			}
		}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-finished:
		return value.value, value.err
	}
}

func configError(output io.Writer, err error) int {
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(output, "perpetual: configuration canceled")
		return 130
	}
	fmt.Fprintf(output, "perpetual: %v\n", err)
	return 1
}
