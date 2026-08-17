package workspace

import (
	"fmt"
	"time"
)

func parseStoredTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("decode stored time: %w", err)
	}
	return parsed, nil
}
