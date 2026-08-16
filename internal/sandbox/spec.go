package sandbox

import "fmt"

type MountMode string

const (
	MountReadOnly  MountMode = "ro"
	MountReadWrite MountMode = "rw"
)

func (m MountMode) Valid() bool {
	return m == MountReadOnly || m == MountReadWrite
}

func (m MountMode) DockerOption() string {
	if m == MountReadOnly {
		return ",readonly"
	}
	return ""
}

func parseMountMode(writable string) (MountMode, error) {
	switch writable {
	case "true":
		return MountReadWrite, nil
	case "false":
		return MountReadOnly, nil
	default:
		return "", fmt.Errorf("invalid Docker workspace mount state %q", writable)
	}
}
