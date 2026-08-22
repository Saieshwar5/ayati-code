package execution

import "errors"

var (
	errNoRuntime = errors.New("execution shell runtime is required")
	errNoRuns    = errors.New("no queued execution rooms")
)
