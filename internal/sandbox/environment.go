package sandbox

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const environmentScript = `set -eu
file=$(mktemp /tmp/perpetual-environment.XXXXXX)
trap 'rm -f "$file"' EXIT HUP INT TERM
chmod 600 "$file"
cat > "$file"
. "$file"
rm -f "$file"
trap - EXIT
exec timeout -k 1 "$1" /bin/sh -c "$2"`

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

func environmentCommand(name, seconds, command string, variables map[string]string) (string, []string) {
	names := make([]string, 0, len(variables))
	for name := range variables {
		names = append(names, name)
	}
	sort.Strings(names)
	var input strings.Builder
	for _, name := range names {
		input.WriteString("export ")
		input.WriteString(name)
		input.WriteByte('=')
		input.WriteString(shellQuote(variables[name]))
		input.WriteByte('\n')
	}
	arguments := []string{"exec", "-i", name, "/bin/sh", "-c", environmentScript, "sh", seconds, command}
	return input.String(), arguments
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func copyVariables(variables map[string]string) map[string]string {
	copy := make(map[string]string, len(variables))
	for name, value := range variables {
		copy[name] = value
	}
	return copy
}

func redactEnvironment(value string, variables map[string]string, truncated bool) string {
	type secret struct{ name, value string }
	secrets := make([]secret, 0, len(variables))
	for name, value := range variables {
		if value != "" {
			secrets = append(secrets, secret{name: name, value: value})
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i].value) > len(secrets[j].value) })
	for _, secret := range secrets {
		value = strings.ReplaceAll(value, secret.value, "[REDACTED:"+secret.name+"]")
		if truncated {
			for length := len(secret.value) - 1; length >= 4; length-- {
				if strings.HasSuffix(value, secret.value[:length]) {
					value = strings.TrimSuffix(value, secret.value[:length]) + "[REDACTED:" + secret.name + "]"
					break
				}
			}
		}
	}
	return value
}
