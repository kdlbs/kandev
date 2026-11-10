// Package modelcatalog owns the provider-neutral presentation structure that
// ACP select options can carry: ordered groups of exact choices plus optional
// verified composition bindings.
//
// The catalog is presentation data only. It never contains executable
// metadata, renderer instructions, inferred pricing, or provider-specific
// interpretation; agents own that meaning and publish it here as bounded data.
package modelcatalog

import (
	"errors"
	"fmt"
	"strings"
)

// SchemaVersion is the only catalog version this build publishes.
const SchemaVersion = 1

// Bounds applied before a catalog is published. Richer provider metadata that
// exceeds any bound is discarded as a whole so the ordinary flat choices keep
// their existing behavior.
const (
	MaxDepth            = 4
	MaxNodes            = 4096
	MaxCompositions     = 64
	MaxDimensions       = 8
	MaxDimensionChoices = 256
	MaxBindings         = 4096
	MaxStringLength     = 512
	MaxIDLength         = 256
)

var (
	// ErrInvalidCatalog reports richer provider metadata that was rejected.
	ErrInvalidCatalog = errors.New("invalid model catalog")
)

// NodeKind distinguishes a group from an exact selectable choice.
type NodeKind string

const (
	NodeKindGroup  NodeKind = "group"
	NodeKindChoice NodeKind = "choice"
)

// Node is one catalog entry. Group nodes carry ordered children; choice nodes
// carry the exact provider value that a selection applies.
type Node struct {
	Kind        NodeKind `json:"kind"`
	ID          string   `json:"id,omitempty"`
	Value       string   `json:"value,omitempty"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Children    []Node   `json:"children,omitempty"`
}

// Catalog is the bounded presentation structure of one select option.
type Catalog struct {
	Version      int           `json:"version"`
	Nodes        []Node        `json:"nodes"`
	Compositions []Composition `json:"compositions,omitempty"`
}

// Choose is one selectable value inside a composition dimension.
type Choose struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Dimension is one independent component of a composed model selection.
type Dimension struct {
	ID      string   `json:"id"`
	Label   string   `json:"label"`
	Choices []Choose `json:"choices"`
}

// Binding maps one complete dimension-value tuple to the exact advertised
// choice value that the provider selects for it.
type Binding struct {
	Values map[string]string `json:"values"`
	Model  string            `json:"model"`
}

// Composition groups the dimensions of one composed model family and the
// verified bindings between them and the advertised catalog leaves.
type Composition struct {
	ID         string      `json:"id"`
	GroupID    string      `json:"group_id"`
	Label      string      `json:"label,omitempty"`
	Dimensions []Dimension `json:"dimensions"`
	Bindings   []Binding   `json:"bindings"`
}

// Choice is one exact advertised leaf without its group path.
type Choice struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// LeafChoices returns the ordered exact choices of a node list. It is the
// compatibility projection used by every existing flat consumer.
func LeafChoices(nodes []Node) []Choice {
	out := make([]Choice, 0, len(nodes))
	var walk func(nodes []Node)
	walk = func(nodes []Node) {
		for _, node := range nodes {
			if node.Kind == NodeKindChoice {
				out = append(out, Choice{Value: node.Value, Label: node.Label, Description: node.Description})
				continue
			}
			walk(node.Children)
		}
	}
	walk(nodes)
	return out
}

// Group returns the first group node with the supplied ID.
func (c *Catalog) Group(groupID string) (*Node, bool) {
	if c == nil {
		return nil, false
	}
	var found *Node
	var walk func(nodes []Node)
	walk = func(nodes []Node) {
		for i := range nodes {
			if found != nil {
				return
			}
			node := &nodes[i]
			if node.Kind == NodeKindGroup && node.ID == groupID {
				found = node
				return
			}
			walk(node.Children)
		}
	}
	walk(c.Nodes)
	return found, found != nil
}

// ChoiceValue reports whether value is an exact advertised leaf.
func (c *Catalog) ChoiceValue(value string) bool {
	for _, choice := range LeafChoices(c.Nodes) {
		if choice.Value == value {
			return true
		}
	}
	return false
}

// Resolve returns the single exact advertised model value for one complete
// dimension tuple. Incomplete, invalid, unknown, and ambiguous tuples resolve
// to ok=false: a caller must never guess another pairing.
func (c *Composition) Resolve(values map[string]string) (string, bool) {
	if c == nil || len(values) != len(c.Dimensions) {
		return "", false
	}
	for _, dimension := range c.Dimensions {
		value, ok := values[dimension.ID]
		if !ok || !hasChoice(dimension.Choices, value) {
			return "", false
		}
	}
	model := ""
	matches := 0
	for _, binding := range c.Bindings {
		if !valuesEqual(binding.Values, values) {
			continue
		}
		matches++
		if matches == 1 {
			model = binding.Model
		}
	}
	if matches != 1 {
		return "", false
	}
	return model, true
}

func hasChoice(choices []Choose, value string) bool {
	for _, choice := range choices {
		if choice.Value == value {
			return true
		}
	}
	return false
}

func valuesEqual(binding, values map[string]string) bool {
	if len(binding) != len(values) {
		return false
	}
	for key, value := range values {
		if binding[key] != value {
			return false
		}
	}
	return true
}

// Validate enforces every catalog bound before publication.
func Validate(catalog *Catalog) error {
	if catalog == nil {
		return fmt.Errorf("%w: nil catalog", ErrInvalidCatalog)
	}
	if catalog.Version != SchemaVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidCatalog, catalog.Version)
	}
	if len(catalog.Nodes) == 0 {
		return fmt.Errorf("%w: empty node list", ErrInvalidCatalog)
	}
	groupIDs := make(map[string]struct{})
	count := 0
	if err := validateNodes(catalog.Nodes, 1, groupIDs, &count); err != nil {
		return err
	}
	return validateCompositions(catalog)
}

func validateNodes(nodes []Node, depth int, groupIDs map[string]struct{}, count *int) error {
	if depth > MaxDepth {
		return fmt.Errorf("%w: depth exceeds %d", ErrInvalidCatalog, MaxDepth)
	}
	for _, node := range nodes {
		*count++
		if *count > MaxNodes {
			return fmt.Errorf("%w: node count exceeds %d", ErrInvalidCatalog, MaxNodes)
		}
		if err := validateText(node.Label); err != nil {
			return err
		}
		if err := validateText(node.Description); err != nil {
			return err
		}
		if err := validateNode(node, depth, groupIDs, count); err != nil {
			return err
		}
	}
	return nil
}

func validateNode(node Node, depth int, groupIDs map[string]struct{}, count *int) error {
	switch node.Kind {
	case NodeKindGroup:
		return validateGroupNode(node, depth, groupIDs, count)
	case NodeKindChoice:
		return validateChoiceNode(node)
	default:
		return fmt.Errorf("%w: unknown node kind %q", ErrInvalidCatalog, node.Kind)
	}
}

func validateGroupNode(node Node, depth int, groupIDs map[string]struct{}, count *int) error {
	if node.ID == "" || len(node.ID) > MaxIDLength {
		return fmt.Errorf("%w: invalid group id", ErrInvalidCatalog)
	}
	if _, exists := groupIDs[node.ID]; exists {
		return fmt.Errorf("%w: duplicate group id %q", ErrInvalidCatalog, node.ID)
	}
	groupIDs[node.ID] = struct{}{}
	if node.Value != "" {
		return fmt.Errorf("%w: group %q carries a value", ErrInvalidCatalog, node.ID)
	}
	if len(node.Children) == 0 {
		return fmt.Errorf("%w: group %q has no children", ErrInvalidCatalog, node.ID)
	}
	return validateNodes(node.Children, depth+1, groupIDs, count)
}

func validateChoiceNode(node Node) error {
	if node.Value == "" || len(node.Value) > MaxIDLength {
		return fmt.Errorf("%w: invalid choice value", ErrInvalidCatalog)
	}
	if node.ID != "" {
		return fmt.Errorf("%w: choice %q carries a group id", ErrInvalidCatalog, node.Value)
	}
	if len(node.Children) > 0 {
		return fmt.Errorf("%w: choice %q carries children", ErrInvalidCatalog, node.Value)
	}
	return nil
}

func validateText(value string) error {
	if len(value) > MaxStringLength {
		return fmt.Errorf("%w: string exceeds %d bytes", ErrInvalidCatalog, MaxStringLength)
	}
	return nil
}

func validateCompositions(catalog *Catalog) error {
	if len(catalog.Compositions) > MaxCompositions {
		return fmt.Errorf("%w: composition count exceeds %d", ErrInvalidCatalog, MaxCompositions)
	}
	seen := make(map[string]struct{}, len(catalog.Compositions))
	for i := range catalog.Compositions {
		if err := validateComposition(&catalog.Compositions[i], catalog, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateComposition(composition *Composition, catalog *Catalog, seen map[string]struct{}) error {
	if composition.ID == "" || len(composition.ID) > MaxIDLength {
		return fmt.Errorf("%w: invalid composition id", ErrInvalidCatalog)
	}
	if err := validateText(composition.Label); err != nil {
		return err
	}
	if _, exists := seen[composition.ID]; exists {
		return fmt.Errorf("%w: duplicate composition id %q", ErrInvalidCatalog, composition.ID)
	}
	seen[composition.ID] = struct{}{}
	if _, ok := catalog.Group(composition.GroupID); !ok {
		return fmt.Errorf("%w: composition %q names an unknown group", ErrInvalidCatalog, composition.ID)
	}
	if err := validateDimensions(composition.Dimensions); err != nil {
		return err
	}
	return validateBindings(composition, catalog)
}

func validateDimensions(dimensions []Dimension) error {
	if len(dimensions) == 0 || len(dimensions) > MaxDimensions {
		return fmt.Errorf("%w: invalid dimension count", ErrInvalidCatalog)
	}
	seen := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		if err := validateDimension(dimension, seen); err != nil {
			return err
		}
	}
	return nil
}

func validateDimension(dimension Dimension, seen map[string]struct{}) error {
	if dimension.ID == "" || len(dimension.ID) > MaxIDLength {
		return fmt.Errorf("%w: invalid dimension id", ErrInvalidCatalog)
	}
	if _, exists := seen[dimension.ID]; exists {
		return fmt.Errorf("%w: duplicate dimension id %q", ErrInvalidCatalog, dimension.ID)
	}
	seen[dimension.ID] = struct{}{}
	if err := validateText(dimension.Label); err != nil {
		return err
	}
	return validateDimensionChoices(dimension)
}

func validateDimensionChoices(dimension Dimension) error {
	if len(dimension.Choices) == 0 || len(dimension.Choices) > MaxDimensionChoices {
		return fmt.Errorf("%w: invalid choice count for dimension %q", ErrInvalidCatalog, dimension.ID)
	}
	values := make(map[string]struct{}, len(dimension.Choices))
	for _, choice := range dimension.Choices {
		if choice.Value == "" || len(choice.Value) > MaxIDLength {
			return fmt.Errorf("%w: invalid choice value for dimension %q", ErrInvalidCatalog, dimension.ID)
		}
		if _, exists := values[choice.Value]; exists {
			return fmt.Errorf("%w: duplicate choice value for dimension %q", ErrInvalidCatalog, dimension.ID)
		}
		values[choice.Value] = struct{}{}
		if err := validateText(choice.Label); err != nil {
			return err
		}
		if err := validateText(choice.Description); err != nil {
			return err
		}
	}
	return nil
}

func validateBindings(composition *Composition, catalog *Catalog) error {
	if len(composition.Bindings) == 0 || len(composition.Bindings) > MaxBindings {
		return fmt.Errorf("%w: invalid binding count", ErrInvalidCatalog)
	}
	tuples := make(map[string]struct{}, len(composition.Bindings))
	for _, binding := range composition.Bindings {
		if len(binding.Values) != len(composition.Dimensions) {
			return fmt.Errorf("%w: binding does not cover every dimension", ErrInvalidCatalog)
		}
		for _, dimension := range composition.Dimensions {
			value, ok := binding.Values[dimension.ID]
			if !ok || !hasChoice(dimension.Choices, value) {
				return fmt.Errorf("%w: binding names an unknown value", ErrInvalidCatalog)
			}
		}
		if !catalog.ChoiceValue(binding.Model) {
			return fmt.Errorf("%w: binding model %q is not advertised", ErrInvalidCatalog, binding.Model)
		}
		key := bindingKey(composition.Dimensions, binding.Values)
		if _, exists := tuples[key]; exists {
			return fmt.Errorf("%w: duplicate binding tuple", ErrInvalidCatalog)
		}
		tuples[key] = struct{}{}
	}
	return nil
}

func bindingKey(dimensions []Dimension, values map[string]string) string {
	var builder strings.Builder
	for _, dimension := range dimensions {
		builder.WriteString(dimension.ID)
		builder.WriteByte('=')
		builder.WriteString(values[dimension.ID])
		builder.WriteByte('\x00')
	}
	return builder.String()
}
