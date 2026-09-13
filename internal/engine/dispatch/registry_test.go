package dispatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeModule is a minimal Module used to exercise the registry without any
// real provider. It records the task/payload it was called with and
// returns a canned Outcome.
type fakeModule struct {
	name string

	gotTask    TaskContext
	gotPayload string
	called     bool

	outcome Outcome
	err     error
}

func (m *fakeModule) Name() string { return m.name }

func (m *fakeModule) Dispatch(_ context.Context, task TaskContext, payload string) (Outcome, error) {
	m.called = true
	m.gotTask = task
	m.gotPayload = payload
	return m.outcome, m.err
}

func TestRegistry_RegisterAndDispatch(t *testing.T) {
	reg := NewRegistry()
	fake := &fakeModule{
		name:    "ios_notifications",
		outcome: Outcome{Accepted: true, Detail: "202 from push service"},
	}
	require.NoError(t, reg.Register(fake))

	task := TaskContext{
		ID:          "buy-milk",
		Title:       "Buy milk",
		FilePath:    "Household/Errands.md",
		State:       "active",
		TriggeredAt: time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC),
	}

	outcome, err := reg.Dispatch(context.Background(), "ios_notifications", task, `{"title":"Buy milk"}`)
	require.NoError(t, err)

	assert.True(t, fake.called, "expected the registered module's Dispatch to be invoked")
	assert.Equal(t, task, fake.gotTask)
	assert.Equal(t, `{"title":"Buy milk"}`, fake.gotPayload)
	assert.Equal(t, Outcome{Accepted: true, Detail: "202 from push service"}, outcome)
}

func TestRegistry_DuplicateRegistrationFails(t *testing.T) {
	reg := NewRegistry()
	first := &fakeModule{name: "ios_notifications"}
	second := &fakeModule{name: "ios_notifications"}

	require.NoError(t, reg.Register(first))
	assert.Error(t, reg.Register(second))

	// The first registration must survive untouched.
	module, err := reg.Lookup("ios_notifications")
	require.NoError(t, err)
	assert.Same(t, Module(first), module, "duplicate registration must not replace the first module")
}

func TestRegistry_LookupUnregisteredNameFails(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Lookup("does_not_exist")
	assert.Error(t, err)

	_, err = reg.Dispatch(context.Background(), "does_not_exist", TaskContext{}, "")
	assert.Error(t, err)
}

func TestRegistry_ModuleErrorPropagates(t *testing.T) {
	reg := NewRegistry()
	wantErr := errors.New("transport failure")
	fake := &fakeModule{name: "ios_notifications", err: wantErr}
	require.NoError(t, reg.Register(fake))

	_, err := reg.Dispatch(context.Background(), "ios_notifications", TaskContext{}, "")
	assert.ErrorIs(t, err, wantErr)
}

func TestRegistry_RegisterNilOrUnnamedModuleFails(t *testing.T) {
	reg := NewRegistry()

	assert.Error(t, reg.Register(nil))
	assert.Error(t, reg.Register(&fakeModule{name: ""}))
}
