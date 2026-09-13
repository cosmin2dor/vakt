package dispatch

import (
	"context"
	"fmt"
	"sync"
)

// Registry holds Modules by name. Safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	modules map[string]Module
}

func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]Module)}
}

// Register adds module under its own Name(). Errors on a nil module, an
// empty name, or a name already registered.
func (r *Registry) Register(module Module) error {
	if module == nil {
		return fmt.Errorf("dispatch: cannot register a nil module")
	}
	name := module.Name()
	if name == "" {
		return fmt.Errorf("dispatch: module has an empty name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("dispatch: module %q is already registered", name)
	}
	r.modules[name] = module
	return nil
}

// Lookup returns the module registered under name, or an error if none is.
func (r *Registry) Lookup(name string) (Module, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, ok := r.modules[name]
	if !ok {
		return nil, fmt.Errorf("dispatch: no module registered under %q", name)
	}
	return module, nil
}

// Dispatch looks up name and invokes its Dispatch method.
func (r *Registry) Dispatch(ctx context.Context, name string, task TaskContext, payload string) (Outcome, error) {
	module, err := r.Lookup(name)
	if err != nil {
		return Outcome{}, err
	}
	return module.Dispatch(ctx, task, payload)
}
