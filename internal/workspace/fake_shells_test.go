package workspace

import (
	"github.com/Saieshwar5/perpetual/internal/exec"
)

// fakeShells records shell creations so tests can assert which workspace
// variables and directories a command would receive without running on the host.
type fakeShells struct {
	shell     exec.Shell
	variables []map[string]string
	dirs      []string
	err       error
}

func (f *fakeShells) open(variables map[string]string, dir string) (exec.Shell, error) {
	f.variables = append(f.variables, variables)
	f.dirs = append(f.dirs, dir)
	return f.shell, f.err
}
