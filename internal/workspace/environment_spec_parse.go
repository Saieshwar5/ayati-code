package workspace

import (
	"encoding/json"
	"sort"
	"strings"
)

type devcontainerConfig struct {
	Features          map[string]any `json:"features"`
	PostCreateCommand any            `json:"postCreateCommand"`
}

func parseToolVersions(data []byte) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			result[strings.ToLower(fields[0])] = strings.TrimSpace(fields[1])
		}
	}
	return result
}

func parseMiseTools(data []byte) map[string]string {
	result := make(map[string]string)
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			section = strings.TrimSpace(strings.Trim(trimmed, "[]"))
			continue
		}
		lowerSection := strings.ToLower(section)
		switch {
		case lowerSection == "tools":
			name, rest, ok := strings.Cut(trimmed, "=")
			if !ok {
				continue
			}
			name = strings.ToLower(strings.TrimSpace(name))
			if version := versionValue(rest); version != "" {
				result[name] = version
			}
		case strings.HasPrefix(lowerSection, "tools."):
			tool := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(section, "tools.")))
			tool = strings.Split(tool, ".")[0]
			name, version, ok := strings.Cut(trimmed, "=")
			if !ok || strings.TrimSpace(name) != "version" {
				continue
			}
			if resolved := versionValue(version); resolved != "" {
				result[tool] = resolved
			}
		}
	}
	return result
}

func versionValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) || strings.HasPrefix(value, `'`) {
		return strings.Trim(value, `"'`)
	}
	if index := strings.Index(value, `"`); index >= 0 {
		rest := value[index+1:]
		if end := strings.Index(rest, `"`); end >= 0 {
			return rest[:end]
		}
	}
	if value == "" || value == "{" || strings.HasSuffix(value, "}") {
		return ""
	}
	return value
}

func parseDevcontainer(data []byte) devcontainerConfig {
	var config devcontainerConfig
	if len(data) == 0 {
		return config
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config
	}
	return config
}

func devcontainerSetupCommand(value any) string {
	switch command := value.(type) {
	case string:
		return strings.TrimSpace(command)
	case []any:
		parts := make([]string, 0, len(command))
		for _, item := range command {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.Join(parts, " && ")
	default:
		return ""
	}
}

func servicesFromDevcontainer(features map[string]any) []DevService {
	known := []struct {
		name  string
		match string
	}{
		{name: "PostgreSQL", match: "postgresql"},
		{name: "MySQL", match: "mysql"},
		{name: "Redis", match: "redis"},
		{name: "MongoDB", match: "mongo"},
		{name: "Docker", match: "docker-in-docker"},
	}
	var services []DevService
	for feature := range features {
		lower := strings.ToLower(feature)
		for _, candidate := range known {
			if strings.Contains(lower, candidate.match) {
				services = append(services, DevService{
					Name: candidate.name, Source: "devcontainer.json",
				})
				break
			}
		}
	}
	seen := make(map[string]bool, len(services))
	result := make([]DevService, 0, len(services))
	for _, service := range services {
		if seen[service.Name] {
			continue
		}
		seen[service.Name] = true
		result = append(result, service)
	}
	return result
}

func appendToolchain(values []Toolchain, name, version, source string) []Toolchain {
	if version == "" && source == "" {
		return values
	}
	for index := range values {
		if values[index].Name != name {
			continue
		}
		if values[index].Version == "" {
			values[index].Version = version
		}
		if values[index].Source == "" {
			values[index].Source = source
		}
		return values
	}
	return append(values, Toolchain{Name: name, Version: version, Source: source})
}

func dedupeToolchains(values []Toolchain) []Toolchain {
	seen := make(map[string]bool, len(values))
	result := make([]Toolchain, 0, len(values))
	for _, current := range values {
		if seen[current.Name] {
			continue
		}
		seen[current.Name] = true
		result = append(result, current)
	}
	return result
}

func sortedMergeToolNames(toolVersions, miseTools map[string]string) []string {
	seen := make(map[string]bool)
	for name := range toolVersions {
		seen[strings.ToLower(name)] = true
	}
	for name := range miseTools {
		seen[strings.ToLower(name)] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mergedToolVersion(toolVersions, miseTools map[string]string, name string) (string, string) {
	name = strings.ToLower(name)
	if version := miseTools[name]; version != "" {
		return version, ".mise.toml"
	}
	if version := toolVersions[name]; version != "" {
		return version, ".tool-versions"
	}
	return "", ""
}

func canonicalToolName(name string) string {
	name = strings.ToLower(name)
	if name == "node" || name == "nodejs" {
		return "Node"
	}
	if name == "python" || name == "python3" {
		return "Python"
	}
	if name == "go" || name == "golang" {
		return "Go"
	}
	if name == "rust" || name == "rustc" || name == "cargo" {
		return "Rust"
	}
	if name == "java" || name == "openjdk" {
		return "Java"
	}
	return name
}

func toolchainVersion(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"Go ", "Node ", "Python "} {
		if strings.HasPrefix(value, prefix) && value != prefix+"(default)" {
			return strings.TrimSpace(strings.TrimPrefix(value, prefix))
		}
	}
	return ""
}

func sumLockfile(files map[string][]byte) string {
	if len(files["go.sum"]) != 0 {
		return "go.sum"
	}
	return ""
}

func nodeToolchain(files map[string][]byte, manifest nodeManifest) (string, string) {
	if value := nodeRuntime(files, manifest); value != "" {
		return toolchainVersion(value), "package.json"
	}
	return "", ""
}

func pythonToolchainSource(files map[string][]byte) string {
	if len(files[".python-version"]) != 0 {
		return ".python-version"
	}
	if len(files["pyproject.toml"]) != 0 {
		return "pyproject.toml"
	}
	return "requirements.txt"
}

func instructionsFile(files map[string][]byte) string {
	if len(files["AGENTS.md"]) != 0 {
		return "AGENTS.md"
	}
	return ""
}
