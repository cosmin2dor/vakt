// Code generated from schema/directives.yaml by cmd/gen-directives. DO NOT EDIT.
package model

// Directive name constants — one per entry in schema/directives.yaml.
const (
	DirectiveId            = "id"
	DirectiveState         = "state"
	DirectiveSchedule      = "schedule"
	DirectiveOnce          = "once"
	DirectiveTarget        = "target"
	DirectiveSkipCount     = "skip_count"
	DirectiveSkipUntil     = "skip_until"
	DirectiveLastTriggered = "last_triggered"
	DirectiveLastCompleted = "last_completed"
	DirectiveReason        = "reason"
)

// DirectiveDescriptor is the generated shape of one schema/directives.yaml
// entry — what GET /api/v1/directives will eventually serve.
type DirectiveDescriptor struct {
	Name          string
	ValueType     string
	Values        []string
	Pattern       *string
	Arity         int
	Required      bool
	SystemWritten bool
	UIHelper      *string
}

// strPtr is a small helper so the literal below can take the address of a
// string constant.
func strPtr(s string) *string { return &s }

// Directives is the full directive registry, in schema/directives.yaml order.
var Directives = []DirectiveDescriptor{
	{
		Name:          "id",
		ValueType:     "string",
		Pattern:       strPtr("^[a-z0-9_-]{1,64}$"),
		Arity:         1,
		Required:      true,
		SystemWritten: false,
		UIHelper:      nil,
	},
	{
		Name:          "state",
		ValueType:     "enum",
		Values:        []string{"active", "triggered", "paused", "completed", "failed"},
		Pattern:       nil,
		Arity:         1,
		Required:      true,
		SystemWritten: false,
		UIHelper:      strPtr("enum_dropdown"),
	},
	{
		Name:          "schedule",
		ValueType:     "cron",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: false,
		UIHelper:      strPtr("cron_popover"),
	},
	{
		Name:          "once",
		ValueType:     "datetime",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: false,
		UIHelper:      strPtr("datetime_picker"),
	},
	{
		Name:          "target",
		ValueType:     "string",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: false,
		UIHelper:      strPtr("enum_dropdown"),
	},
	{
		Name:          "skip_count",
		ValueType:     "integer",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: false,
		UIHelper:      nil,
	},
	{
		Name:          "skip_until",
		ValueType:     "datetime",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: false,
		UIHelper:      strPtr("datetime_picker"),
	},
	{
		Name:          "last_triggered",
		ValueType:     "datetime",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: true,
		UIHelper:      nil,
	},
	{
		Name:          "last_completed",
		ValueType:     "datetime",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: true,
		UIHelper:      nil,
	},
	{
		Name:          "reason",
		ValueType:     "string",
		Pattern:       nil,
		Arity:         1,
		Required:      false,
		SystemWritten: false,
		UIHelper:      nil,
	},
}
