package exec

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

type secret struct{ name, value string }

// validateVariables rejects names that cannot be exported and values that
// contain NUL bytes.
func validateVariables(variables map[string]string) error {
	for name, value := range variables {
		if name == "" || (name[0] != '_' && (name[0] < 'A' || name[0] > 'Z') && (name[0] < 'a' || name[0] > 'z')) {
			return fmt.Errorf("invalid environment variable name %q", name)
		}
		for _, character := range name[1:] {
			if character != '_' && (character < 'A' || character > 'Z') &&
				(character < 'a' || character > 'z') && (character < '0' || character > '9') {
				return fmt.Errorf("invalid environment variable name %q", name)
			}
		}
		if strings.ContainsRune(value, '\x00') {
			return errors.New("environment variable contains a NUL byte")
		}
	}
	return nil
}

func copyVariables(variables map[string]string) map[string]string {
	copy := make(map[string]string, len(variables))
	for name, value := range variables {
		copy[name] = value
	}
	return copy
}

func sortStrings(values []string) {
	sort.Strings(values)
}

// redactEnvironment replaces configured values in captured output.
func redactEnvironment(value string, variables map[string]string, truncated bool) string {
	secrets := make([]secret, 0, len(variables))
	for name, value := range variables {
		if value != "" {
			secrets = append(secrets, secret{name: name, value: value})
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i].value) > len(secrets[j].value) })
	for _, current := range secrets {
		value = strings.ReplaceAll(value, current.value, "[REDACTED:"+current.name+"]")
		if truncated {
			for length := len(current.value) - 1; length >= 4; length-- {
				if strings.HasSuffix(value, current.value[:length]) {
					value = strings.TrimSuffix(value, current.value[:length]) + "[REDACTED:" + current.name + "]"
					break
				}
			}
		}
	}
	return value
}
