// Command gen-directives reads schema/directives.yaml and emits the
// generated Go (internal/model/directives_gen.go) and TypeScript
// (web/src/lib/directives-gen.ts) registry constants (SDD.md §2.5,
// generate-registry-constants). Invoked by scripts/generate.sh, not run
// directly.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// directive mirrors one entry of schema/directives.yaml — see that
// file's header comment for the field reference.
type directive struct {
	Name          string   `yaml:"name"`
	ValueType     string   `yaml:"value_type"`
	Values        []string `yaml:"values"`
	Pattern       string   `yaml:"pattern"`
	Arity         int      `yaml:"arity"`
	Required      bool     `yaml:"required"`
	SystemWritten bool     `yaml:"system_written"`
	UIHelper      *string  `yaml:"ui_helper"`
}

type registry struct {
	Directives []directive `yaml:"directives"`
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: gen-directives <directives.yaml> <out.go> <out.ts>")
		os.Exit(1)
	}
	yamlPath, goPath, tsPath := os.Args[1], os.Args[2], os.Args[3]

	data, err := os.ReadFile(yamlPath)
	if err != nil {
		fatal(err)
	}
	var reg registry
	if err := yaml.Unmarshal(data, &reg); err != nil {
		fatal(err)
	}

	if err := os.WriteFile(goPath, []byte(renderGo(reg.Directives)), 0o644); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(tsPath, []byte(renderTS(reg.Directives)), 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen-directives:", err)
	os.Exit(1)
}

// constName turns "skip_count" into "DirectiveSkipCount".
func constName(directiveName string) string {
	parts := strings.Split(directiveName, "_")
	var b strings.Builder
	b.WriteString("Directive")
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
}

func goStringSlice(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	return "[]string{" + strings.Join(quoted, ", ") + "}"
}

func goStringPtr(s string) string {
	if s == "" {
		return "nil"
	}
	return "strPtr(" + strconv.Quote(s) + ")"
}

func goUIHelper(h *string) string {
	if h == nil {
		return "nil"
	}
	return goStringPtr(*h)
}

func renderGo(directives []directive) string {
	var b strings.Builder
	b.WriteString("// Code generated from schema/directives.yaml by cmd/gen-directives. DO NOT EDIT.\n")
	b.WriteString("package model\n\n")

	b.WriteString("// Directive name constants — one per entry in schema/directives.yaml.\n")
	b.WriteString("const (\n")
	for _, d := range directives {
		fmt.Fprintf(&b, "\t%s = %s\n", constName(d.Name), strconv.Quote(d.Name))
	}
	b.WriteString(")\n\n")

	b.WriteString("// DirectiveDescriptor is the generated shape of one schema/directives.yaml\n")
	b.WriteString("// entry — what GET /api/v1/directives will eventually serve.\n")
	b.WriteString("type DirectiveDescriptor struct {\n")
	b.WriteString("\tName          string\n")
	b.WriteString("\tValueType     string\n")
	b.WriteString("\tValues        []string\n")
	b.WriteString("\tPattern       *string\n")
	b.WriteString("\tArity         int\n")
	b.WriteString("\tRequired      bool\n")
	b.WriteString("\tSystemWritten bool\n")
	b.WriteString("\tUIHelper      *string\n")
	b.WriteString("}\n\n")

	b.WriteString("// strPtr is a small helper so the literal below can take the address of a\n")
	b.WriteString("// string constant.\n")
	b.WriteString("func strPtr(s string) *string { return &s }\n\n")

	b.WriteString("// Directives is the full directive registry, in schema/directives.yaml order.\n")
	b.WriteString("var Directives = []DirectiveDescriptor{\n")
	for _, d := range directives {
		b.WriteString("\t{\n")
		fmt.Fprintf(&b, "\t\tName:          %s,\n", strconv.Quote(d.Name))
		fmt.Fprintf(&b, "\t\tValueType:     %s,\n", strconv.Quote(d.ValueType))
		if len(d.Values) > 0 {
			fmt.Fprintf(&b, "\t\tValues:        %s,\n", goStringSlice(d.Values))
		}
		fmt.Fprintf(&b, "\t\tPattern:       %s,\n", goStringPtr(d.Pattern))
		fmt.Fprintf(&b, "\t\tArity:         %d,\n", d.Arity)
		fmt.Fprintf(&b, "\t\tRequired:      %t,\n", d.Required)
		fmt.Fprintf(&b, "\t\tSystemWritten: %t,\n", d.SystemWritten)
		fmt.Fprintf(&b, "\t\tUIHelper:      %s,\n", goUIHelper(d.UIHelper))
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")

	return b.String()
}

func tsConstName(directiveName string) string {
	return strings.ToUpper(constName(directiveName))
}

func tsStringLit(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}

func tsNullableString(s string) string {
	if s == "" {
		return "null"
	}
	return tsStringLit(s)
}

func tsValues(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = tsStringLit(v)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func tsUIHelper(h *string) string {
	if h == nil {
		return "null"
	}
	return tsStringLit(*h)
}

func renderTS(directives []directive) string {
	var b strings.Builder
	b.WriteString("/**\n")
	b.WriteString(" * Generated from schema/directives.yaml by cmd/gen-directives. Do not edit.\n")
	b.WriteString(" */\n\n")

	b.WriteString("export interface DirectiveDescriptor {\n")
	b.WriteString("  name: string\n")
	b.WriteString("  valueType: string\n")
	b.WriteString("  values: string[]\n")
	b.WriteString("  pattern: string | null\n")
	b.WriteString("  arity: number\n")
	b.WriteString("  required: boolean\n")
	b.WriteString("  systemWritten: boolean\n")
	b.WriteString("  uiHelper: string | null\n")
	b.WriteString("}\n\n")

	for _, d := range directives {
		fmt.Fprintf(&b, "export const %s = %s\n", tsConstName(d.Name), tsStringLit(d.Name))
	}
	b.WriteString("\n")

	b.WriteString("export const DIRECTIVES: DirectiveDescriptor[] = [\n")
	for _, d := range directives {
		b.WriteString("  {\n")
		fmt.Fprintf(&b, "    name: %s,\n", tsStringLit(d.Name))
		fmt.Fprintf(&b, "    valueType: %s,\n", tsStringLit(d.ValueType))
		fmt.Fprintf(&b, "    values: %s,\n", tsValues(d.Values))
		fmt.Fprintf(&b, "    pattern: %s,\n", tsNullableString(d.Pattern))
		fmt.Fprintf(&b, "    arity: %d,\n", d.Arity)
		fmt.Fprintf(&b, "    required: %t,\n", d.Required)
		fmt.Fprintf(&b, "    systemWritten: %t,\n", d.SystemWritten)
		fmt.Fprintf(&b, "    uiHelper: %s,\n", tsUIHelper(d.UIHelper))
		b.WriteString("  },\n")
	}
	b.WriteString("]\n")

	return b.String()
}
