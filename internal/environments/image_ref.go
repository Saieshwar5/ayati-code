package environments

import "strings"

// ParseImageRef parses an artifact reference written as
// "lambda:<imageArn>:<version>" and returns the parts.
func ParseImageRef(ref string) (imageARN, version string, ok bool) {
	prefix := "lambda:"
	if !strings.HasPrefix(ref, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(ref, prefix)
	index := strings.LastIndex(rest, ":")
	if index <= 0 || index == len(rest)-1 {
		return "", "", false
	}
	return rest[:index], rest[index+1:], true
}
