package workspaceruntime

import "context"

// Reconciler is implemented by runtimes that own external instances (for
// example Lambda MicroVMs) and can refresh/clean their persisted records.
type Reconciler interface {
	Reconcile(context.Context) error
}
