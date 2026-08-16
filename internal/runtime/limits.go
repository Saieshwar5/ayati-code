package runtime

import "time"

func normalizedLimits(limits Limits) Limits {
	if limits.MaxSteps <= 0 {
		limits.MaxSteps = 30
	}
	if limits.MaxContextRollovers <= 0 {
		limits.MaxContextRollovers = 2
	}
	if limits.RunTimeout <= 0 {
		limits.RunTimeout = 30 * time.Minute
	}
	if limits.ModelTimeout <= 0 {
		limits.ModelTimeout = 5 * time.Minute
	}
	if limits.ShellTimeout <= 0 {
		limits.ShellTimeout = 2 * time.Minute
	}
	if limits.MaxOutputBytes <= 0 {
		limits.MaxOutputBytes = 16 << 10
	}
	return limits
}

func snapshotLimits(limits Limits, policy ContextPolicy) *LimitSnapshot {
	return &LimitSnapshot{
		MaxSteps: limits.MaxSteps, MaxContextRollovers: limits.MaxContextRollovers,
		ContextWindowTokens: policy.WindowTokens, ContextCheckpointTokens: contextCheckpointTokens(policy),
		RunTimeoutMS: limits.RunTimeout.Milliseconds(), ModelTimeoutMS: limits.ModelTimeout.Milliseconds(),
		ShellTimeoutMS: limits.ShellTimeout.Milliseconds(), MaxOutputBytes: limits.MaxOutputBytes,
	}
}
