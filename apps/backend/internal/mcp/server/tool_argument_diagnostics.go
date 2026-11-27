package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

const maxToolDiagnosticDetails = 8
const maxToolDiagnosticBytes = 4096

type toolDiagnostics struct {
	schemas  map[string]*jsonschema.Schema
	instance any
	details  []string
	omitted  bool
}

// Schema locations preserve alternative identity without interpreting failed
// alternatives as simultaneous requirements.
func diagnosticSchemaIndex(root *jsonschema.Schema) map[string]*jsonschema.Schema {
	index := make(map[string]*jsonschema.Schema)
	var visit func(*jsonschema.Schema)
	visit = func(s *jsonschema.Schema) {
		if s == nil {
			return
		}
		if _, ok := index[s.Location]; ok {
			return
		}
		index[s.Location] = s
		for _, child := range diagnosticSchemaChildren(s) {
			visit(child)
		}
	}
	visit(root)
	return index
}

func (d *toolDiagnostics) add(detail string) {
	if len(d.details) < maxToolDiagnosticDetails {
		d.details = append(d.details, detail)
	} else {
		d.omitted = true
	}
}

func (d *toolDiagnostics) project(failure *jsonschema.ValidationError) *jsonschema.ValidationError {
	if failure == nil {
		return nil
	}
	projected := *failure
	projected.Causes = nil
	schema := d.schemas[failure.SchemaURL]
	branches := diagnosticUnionBranches(failure, schema)
	if len(branches) > 0 {
		return d.projectUnion(failure, branches)
	}
	for _, cause := range failure.Causes {
		projected.Causes = append(projected.Causes, d.project(cause))
	}
	d.describeFailure(failure, schema)
	return &projected
}

func diagnosticUnionBranches(failure *jsonschema.ValidationError, s *jsonschema.Schema) []*jsonschema.Schema {
	if s == nil {
		return nil
	}
	switch failure.ErrorKind.(type) {
	case *kind.OneOf:
		return s.OneOf
	case *kind.AnyOf:
		return s.AnyOf
	}
	return nil
}

func (d *toolDiagnostics) projectUnion(failure *jsonschema.ValidationError, branches []*jsonschema.Schema) *jsonschema.ValidationError {
	projected := *failure
	projected.Causes = nil
	if one, ok := failure.ErrorKind.(*kind.OneOf); ok && len(one.Subschemas) > 1 {
		d.add("multiple alternatives match at " + validationInstancePath(failure.InstanceLocation))
		return &projected
	}
	value, _ := diagnosticInstance(d.instance, failure.InstanceLocation).(map[string]any)
	selected, selector := selectDiagnosticBranch(branches, value)
	if selected >= 0 {
		projected.ErrorKind = &kind.Group{}
		for _, cause := range failure.Causes {
			if cause.SchemaURL == branches[selected].Location || strings.HasPrefix(cause.SchemaURL, branches[selected].Location+"/") {
				projected.Causes = append(projected.Causes, d.project(cause))
			}
		}
		return &projected
	}
	if selector != "" {
		choices := make([]string, 0, len(branches))
		for _, branch := range branches {
			choices = append(choices, diagnosticLiteral(*branch.Properties[selector].Const))
		}
		d.add("choose " + strings.Join(choices, ", ") + " at " + validationInstancePath(append(append([]string{}, failure.InstanceLocation...), selector)))
		return &projected
	}
	for i, branch := range branches {
		for _, cause := range failure.Causes {
			if cause.SchemaURL == branch.Location || strings.HasPrefix(cause.SchemaURL, branch.Location+"/") {
				alternative := &toolDiagnostics{schemas: d.schemas, instance: d.instance}
				tree := alternative.project(cause)
				path := validationInstancePath(failure.InstanceLocation)
				detail := fmt.Sprintf("alternative %d at %s", i+1, path) + missingRequiredProperties(tree, path) + unknownArgumentDetail("", tree, path)
				if len(alternative.details) > 0 {
					detail += "; " + strings.Join(alternative.details, "; ")
				}
				d.add(detail)
			}
		}
	}
	return &projected
}

func selectDiagnosticBranch(branches []*jsonschema.Schema, value map[string]any) (int, string) {
	selector := diagnosticSelector(branches)
	if selector != "" {
		for i, branch := range branches {
			if value[selector] == *branch.Properties[selector].Const {
				return i, selector
			}
		}
		return -1, selector
	}
	counts := make(map[string]int)
	for _, branch := range branches {
		for _, key := range branch.Required {
			counts[key]++
		}
	}
	selected := -1
	for i, branch := range branches {
		for _, key := range branch.Required {
			if _, present := value[key]; present && counts[key] == 1 {
				if selected >= 0 && selected != i {
					return -1, ""
				}
				selected = i
				break
			}
		}
	}
	return selected, ""
}

func diagnosticSelector(branches []*jsonschema.Schema) string {
	keys := make([]string, 0, len(branches[0].Properties))
	for key := range branches[0].Properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values := make(map[string]bool)
		for _, branch := range branches {
			property := branch.Properties[key]
			if property == nil || property.Const == nil {
				break
			}
			value, ok := (*property.Const).(string)
			if !ok {
				break
			}
			values[value] = true
		}
		if len(values) == len(branches) {
			return key
		}
	}
	return ""
}

func diagnosticInstance(instance any, path []string) any {
	for _, token := range path {
		switch value := instance.(type) {
		case map[string]any:
			instance = value[token]
		case []any:
			i, err := strconv.Atoi(token)
			if err != nil || i < 0 || i >= len(value) {
				return nil
			}
			instance = value[i]
		default:
			return nil
		}
	}
	return instance
}

func (d *toolDiagnostics) describeFailure(failure *jsonschema.ValidationError, schema *jsonschema.Schema) {
	path := validationInstancePath(failure.InstanceLocation)
	switch k := failure.ErrorKind.(type) {
	case *kind.Type:
		detail := "expected " + strings.Join(k.Want, " or ") + " at " + path
		if schema != nil && len(schema.Required) > 0 {
			detail += " with required fields " + strings.Join(schema.Required, ", ")
		}
		d.add(detail)
	case *kind.Enum:
		choices := make([]string, 0, len(k.Want))
		for _, v := range k.Want {
			choices = append(choices, diagnosticLiteral(v))
		}
		d.add("expected one of " + strings.Join(choices, ", ") + " at " + path)
	case *kind.Const:
		d.add("expected " + diagnosticLiteral(k.Want) + " at " + path)
	}
}

func diagnosticLiteral(value any) string {
	switch value.(type) {
	case string, bool, float64, int, json.Number, nil:
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
	}
	return "schema-defined value"
}

func boundedToolDiagnostic(message string) string {
	const suffix = "; additional diagnostics omitted"
	if len(message) <= maxToolDiagnosticBytes {
		return message
	}
	limit := maxToolDiagnosticBytes - len(suffix)
	for !utf8.ValidString(message[:limit]) {
		limit--
	}
	return message[:limit] + suffix
}

func diagnosticSchemaChildren(s *jsonschema.Schema) []*jsonschema.Schema {
	children := []*jsonschema.Schema{s.Ref, s.RecursiveRef, s.Not, s.If, s.Then, s.Else, s.PropertyNames, s.UnevaluatedProperties, s.Items2020, s.Contains, s.UnevaluatedItems, s.ContentSchema}
	for _, group := range [][]*jsonschema.Schema{s.AllOf, s.AnyOf, s.OneOf, s.PrefixItems} {
		children = append(children, group...)
	}
	for _, group := range []map[string]*jsonschema.Schema{s.Properties, s.DependentSchemas} {
		for _, child := range group {
			children = append(children, child)
		}
	}
	for _, child := range s.PatternProperties {
		children = append(children, child)
	}
	if child, ok := s.AdditionalItems.(*jsonschema.Schema); ok {
		children = append(children, child)
	}
	if child, ok := s.AdditionalProperties.(*jsonschema.Schema); ok {
		children = append(children, child)
	}
	switch items := s.Items.(type) {
	case *jsonschema.Schema:
		children = append(children, items)
	case []*jsonschema.Schema:
		children = append(children, items...)
	}
	for _, dependency := range s.Dependencies {
		if child, ok := dependency.(*jsonschema.Schema); ok {
			children = append(children, child)
		}
	}
	return children
}
