package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const EnvPath = "AYATI_MICRO_ENV"

var KnownKeys = []string{
	"FIREWORKS_API_KEY",
	"NCA_MODEL",
	"NCA_CONTEXT_PERCENT",
	"NCA_MODEL_CONTEXT_TOKENS",
	"NCA_MAX_CONTEXT_TOOL_PAIRS",
	"NCA_MAX_TOOL_CALLS",
	"NCA_SHELL_TIMEOUT",
	"NCA_MAX_OUTPUT",
	"NCA_SESSION_DIR",
	"NCA_FIREWORKS_URL",
}

type Values map[string]string

func Path() (string, error) {
	if path := os.Getenv(EnvPath); path != "" {
		return filepath.Abs(path)
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find ayati-micro executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve ayati-micro executable: %w", err)
	}
	return filepath.Join(filepath.Dir(resolved), ".env"), nil
}

func Load(path string) (Values, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return Values{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	values := Values{}
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, rawValue, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !found || !isKnown(key) {
			return nil, fmt.Errorf("invalid config entry at %s:%d", path, lineNumber)
		}
		value, err := parseValue(strings.TrimSpace(rawValue))
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s at %s:%d: %w", key, path, lineNumber, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	return values, nil
}

func Save(path string, values Values) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".env-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("secure config permissions: %w", err)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if isKnown(key) {
			keys = append(keys, key)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool { return keyOrder(keys[i]) < keyOrder(keys[j]) })
	for _, key := range keys {
		if _, err := fmt.Fprintf(file, "%s=%s\n", key, strconv.Quote(values[key])); err != nil {
			file.Close()
			return fmt.Errorf("write config: %w", err)
		}
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync config: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func Effective(values Values, key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}
	if value := values[key]; value != "" {
		return value
	}
	return fallback
}

func Mask(value string) string {
	if value == "" {
		return "not configured"
	}
	if len(value) <= 8 {
		return "configured (********)"
	}
	return "configured (" + value[:3] + "****" + value[len(value)-4:] + ")"
}

func parseValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] == '\'' {
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", fmt.Errorf("unterminated single quote")
		}
		return value[1 : len(value)-1], nil
	}
	if value[0] == '"' {
		parsed, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return parsed, nil
	}
	return value, nil
}

func isKnown(key string) bool {
	for _, known := range KnownKeys {
		if key == known {
			return true
		}
	}
	return false
}

func keyOrder(key string) int {
	for index, known := range KnownKeys {
		if key == known {
			return index
		}
	}
	return len(KnownKeys)
}
