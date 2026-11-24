package acpcompat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

const (
	// DevinFusionUIDPrefix and DevinFusionLabelPrefix identify the qualified
	// native Fusion shape. A Fusion record is never a candidate Lead or
	// Sidekick, so a nested pairing cannot invent a third dimension.
	DevinFusionUIDPrefix   = "fusion-"
	DevinFusionLabelPrefix = "Fusion ("
	// DevinSidekickUIDInfix is the only separator the native Fusion identity
	// uses between the Lead and Sidekick variant identities.
	DevinSidekickUIDInfix = "-sidekick-"

	// DevinLeadDimensionID and DevinSidekickDimensionID are the presentation
	// dimensions of a qualified native Fusion pairing.
	DevinLeadDimensionID     = "lead"
	DevinSidekickDimensionID = "sidekick"

	// MaxDevinCatalogBytes bounds one native listing before any decoding.
	MaxDevinCatalogBytes = 2 << 20
	// MaxDevinFamilies and MaxDevinVariants bound the decoded structure.
	MaxDevinFamilies          = 512
	MaxDevinVariants          = 4096
	MaxDevinVariantsPerFamily = 1024
)

// ErrInvalidDevinCatalog reports native Devin catalog data this build cannot
// qualify. Callers keep the ordinary ACP catalog on this error.
var ErrInvalidDevinCatalog = errors.New("invalid devin native model catalog")

// DevinNativeVariant is one native model variant: the exact Sidekick identity
// and part of the Lead variant identity.
type DevinNativeVariant struct {
	UID   string `json:"model_uid"`
	Label string `json:"label"`
}

// DevinNativeFamily is one native model family: the Lead identity.
type DevinNativeFamily struct {
	UID      string               `json:"family_uid"`
	Label    string               `json:"family_label"`
	Variants []DevinNativeVariant `json:"variants"`
}

// DevinNativeCatalog is the bounded relationship evidence decoded from
// `devin models list --format json`. It supplies bindings only; it never adds
// availability to the ACP catalog.
type DevinNativeCatalog struct {
	Families []DevinNativeFamily `json:"families"`
}

// ParseDevinNativeCatalog decodes and validates one native listing. Duplicate
// or malformed identities are rejected as a whole rather than guessed around.
func ParseDevinNativeCatalog(data []byte) (*DevinNativeCatalog, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrInvalidDevinCatalog)
	}
	if len(data) > MaxDevinCatalogBytes {
		return nil, fmt.Errorf("%w: payload exceeds %d bytes", ErrInvalidDevinCatalog, MaxDevinCatalogBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var native DevinNativeCatalog
	if err := decoder.Decode(&native); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidDevinCatalog, err)
	}
	if err := expectJSONEnd(decoder); err != nil {
		return nil, err
	}
	if err := native.validate(); err != nil {
		return nil, err
	}
	return &native, nil
}

func expectJSONEnd(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("%w: trailing data", ErrInvalidDevinCatalog)
	}
	return nil
}

func (n *DevinNativeCatalog) validate() error {
	if n == nil || len(n.Families) == 0 {
		return fmt.Errorf("%w: no families", ErrInvalidDevinCatalog)
	}
	if len(n.Families) > MaxDevinFamilies {
		return fmt.Errorf("%w: family count exceeds %d", ErrInvalidDevinCatalog, MaxDevinFamilies)
	}
	familyUIDs := make(map[string]struct{}, len(n.Families))
	variantUIDs := make(map[string]struct{}, len(n.Families))
	total := 0
	for _, family := range n.Families {
		if err := validateDevinText(family.UID, modelcatalog.MaxIDLength); err != nil {
			return fmt.Errorf("%w: family identifier: %v", ErrInvalidDevinCatalog, err)
		}
		if err := validateDevinText(family.Label, modelcatalog.MaxStringLength); err != nil {
			return fmt.Errorf("%w: family label: %v", ErrInvalidDevinCatalog, err)
		}
		if _, exists := familyUIDs[family.UID]; exists {
			return fmt.Errorf("%w: duplicate family %q", ErrInvalidDevinCatalog, family.UID)
		}
		familyUIDs[family.UID] = struct{}{}
		if len(family.Variants) == 0 || len(family.Variants) > MaxDevinVariantsPerFamily {
			return fmt.Errorf("%w: invalid variant count for %q", ErrInvalidDevinCatalog, family.UID)
		}
		total += len(family.Variants)
		if total > MaxDevinVariants {
			return fmt.Errorf("%w: variant count exceeds %d", ErrInvalidDevinCatalog, MaxDevinVariants)
		}
		if err := validateDevinVariants(family.Variants, variantUIDs); err != nil {
			return err
		}
	}
	return nil
}

func validateDevinVariants(variants []DevinNativeVariant, seen map[string]struct{}) error {
	for _, variant := range variants {
		if err := validateDevinText(variant.UID, modelcatalog.MaxIDLength); err != nil {
			return fmt.Errorf("%w: variant identifier: %v", ErrInvalidDevinCatalog, err)
		}
		if err := validateDevinText(variant.Label, modelcatalog.MaxStringLength); err != nil {
			return fmt.Errorf("%w: variant label: %v", ErrInvalidDevinCatalog, err)
		}
		if _, exists := seen[variant.UID]; exists {
			return fmt.Errorf("%w: duplicate variant %q", ErrInvalidDevinCatalog, variant.UID)
		}
		seen[variant.UID] = struct{}{}
	}
	return nil
}

func validateDevinText(value string, limit int) error {
	if value == "" {
		return errors.New("empty value")
	}
	if len(value) > limit {
		return fmt.Errorf("value exceeds %d bytes", limit)
	}
	return nil
}

type devinVariantRef struct {
	UID         string
	Label       string
	FamilyUID   string
	FamilyLabel string
}

type devinPair struct {
	lead     devinVariantRef
	sidekick devinVariantRef
}

type devinIndex struct {
	variants map[string]devinVariantRef
}

func (n *DevinNativeCatalog) index() devinIndex {
	index := devinIndex{variants: make(map[string]devinVariantRef, MaxDevinVariants)}
	if n == nil {
		return index
	}
	for _, family := range n.Families {
		for _, variant := range family.Variants {
			if _, exists := index.variants[variant.UID]; exists {
				continue
			}
			index.variants[variant.UID] = devinVariantRef{
				UID:         variant.UID,
				Label:       variant.Label,
				FamilyUID:   family.UID,
				FamilyLabel: family.Label,
			}
		}
	}
	return index
}

func isDevinFusionShaped(uid, label string) bool {
	return strings.HasPrefix(uid, DevinFusionUIDPrefix) &&
		strings.HasPrefix(label, DevinFusionLabelPrefix)
}

// DevinFusionLabel is the exact native label expression for one pair.
func DevinFusionLabel(leadLabel, sidekickLabel string) string {
	return DevinFusionLabelPrefix + leadLabel + " + " + sidekickLabel + ")"
}

// resolveFusion returns every native pair that the exact identity and label
// expressions explain. More than one pair is ambiguous and never becomes a
// binding.
func (index devinIndex) resolveFusion(uid, label string) []devinPair {
	if !isDevinFusionShaped(uid, label) {
		return nil
	}
	rest := strings.TrimPrefix(uid, DevinFusionUIDPrefix)
	var pairs []devinPair
	for offset := 0; offset < len(rest); {
		at := strings.Index(rest[offset:], DevinSidekickUIDInfix)
		if at < 0 {
			break
		}
		at += offset
		if pair, ok := index.resolveFusionSplit(rest[:at], rest[at+len(DevinSidekickUIDInfix):], label); ok {
			pairs = append(pairs, pair)
		}
		offset = at + 1
	}
	return pairs
}

func (index devinIndex) resolveFusionSplit(leadUID, sidekickUID, label string) (devinPair, bool) {
	if leadUID == "" || sidekickUID == "" {
		return devinPair{}, false
	}
	lead, leadOK := index.variants[leadUID]
	sidekick, sidekickOK := index.variants[sidekickUID]
	if !leadOK || !sidekickOK {
		return devinPair{}, false
	}
	if isDevinFusionShaped(lead.UID, lead.Label) || isDevinFusionShaped(sidekick.UID, sidekick.Label) {
		return devinPair{}, false
	}
	if label != DevinFusionLabel(lead.Label, sidekick.Label) {
		return devinPair{}, false
	}
	return devinPair{lead: lead, sidekick: sidekick}, true
}

type devinAcceptedBinding struct {
	model    string
	lead     devinVariantRef
	sidekick devinVariantRef
}

// EnrichDevinCatalog returns a validated copy of catalog with one qualified
// Fusion composition per group whose exact advertised leaves the native
// evidence explains. The original catalog and its flat projection stay
// untouched, and any unusable enrichment returns the original catalog.
func EnrichDevinCatalog(catalog *modelcatalog.Catalog, native *DevinNativeCatalog) *modelcatalog.Catalog {
	if catalog == nil || native == nil {
		return catalog
	}
	// Native evidence can arrive from a bounded parser or an injected runner
	// fixture; reject invalid identities before the index can pick a first match.
	if err := native.validate(); err != nil {
		return catalog
	}
	index := native.index()
	if len(index.variants) == 0 {
		return catalog
	}
	enriched := cloneDevinCatalog(catalog)
	claimed := make(map[string]struct{}, len(enriched.Compositions))
	for _, composition := range enriched.Compositions {
		claimed[composition.GroupID] = struct{}{}
	}
	before := len(enriched.Compositions)
	appendDevinCompositions(enriched.Nodes, index, claimed, &enriched.Compositions)
	if len(enriched.Compositions) == before {
		return catalog
	}
	if err := modelcatalog.Validate(enriched); err != nil {
		return catalog
	}
	return enriched
}

func appendDevinCompositions(
	nodes []modelcatalog.Node,
	index devinIndex,
	claimed map[string]struct{},
	out *[]modelcatalog.Composition,
) {
	for i := range nodes {
		node := &nodes[i]
		if node.Kind != modelcatalog.NodeKindGroup {
			continue
		}
		if _, exists := claimed[node.ID]; !exists {
			if composition, ok := devinCompositionForGroup(node, index); ok {
				*out = append(*out, composition)
				claimed[node.ID] = struct{}{}
			}
		}
		appendDevinCompositions(node.Children, index, claimed, out)
	}
}

func devinCompositionForGroup(
	group *modelcatalog.Node,
	index devinIndex,
) (modelcatalog.Composition, bool) {
	accepted := acceptedDevinBindings(group, index)
	if len(accepted) == 0 {
		return modelcatalog.Composition{}, false
	}
	leadChoices, sidekickChoices := devinDimensionChoices(accepted)
	if len(leadChoices) == 0 || len(sidekickChoices) == 0 {
		return modelcatalog.Composition{}, false
	}
	bindings := make([]modelcatalog.Binding, 0, len(accepted))
	for _, binding := range accepted {
		bindings = append(bindings, modelcatalog.Binding{
			Values: map[string]string{
				DevinLeadDimensionID:     binding.lead.FamilyUID,
				DevinSidekickDimensionID: binding.sidekick.UID,
			},
			Model: binding.model,
		})
	}
	return modelcatalog.Composition{
		ID:      group.ID,
		GroupID: group.ID,
		Label:   group.Label,
		Dimensions: []modelcatalog.Dimension{
			{ID: DevinLeadDimensionID, Label: "Lead", Choices: leadChoices},
			{ID: DevinSidekickDimensionID, Label: "Sidekick", Choices: sidekickChoices},
		},
		Bindings: bindings,
	}, true
}

// acceptedDevinBindings joins native evidence to exact advertised leaves, then
// drops every advertised value whose tuple is not unique. Duplicate leaves and
// ambiguous pairs never receive a binding.
func acceptedDevinBindings(group *modelcatalog.Node, index devinIndex) []devinAcceptedBinding {
	counts := catalogLeafCounts(group.Children)
	var accepted []devinAcceptedBinding
	for _, child := range group.Children {
		if child.Kind != modelcatalog.NodeKindChoice {
			continue
		}
		// A composition renders one configuration entry and hides its bound
		// leaves, so a repeated advertised value would lose its other rows.
		if counts[child.Value] > 1 {
			continue
		}
		native, ok := index.variants[child.Value]
		if !ok || native.Label != child.Label {
			continue
		}
		pairs := index.resolveFusion(native.UID, native.Label)
		if len(pairs) != 1 {
			continue
		}
		accepted = append(accepted, devinAcceptedBinding{
			model:    child.Value,
			lead:     pairs[0].lead,
			sidekick: pairs[0].sidekick,
		})
	}
	return uniqueDevinTuples(accepted)
}

func catalogLeafCounts(children []modelcatalog.Node) map[string]int {
	counts := make(map[string]int, len(children))
	for _, child := range children {
		if child.Kind == modelcatalog.NodeKindChoice {
			counts[child.Value]++
		}
	}
	return counts
}

func uniqueDevinTuples(accepted []devinAcceptedBinding) []devinAcceptedBinding {
	tuples := make(map[string]int, len(accepted))
	for _, binding := range accepted {
		tuples[devinTupleKey(binding)]++
	}
	unique := make([]devinAcceptedBinding, 0, len(accepted))
	for _, binding := range accepted {
		if tuples[devinTupleKey(binding)] == 1 {
			unique = append(unique, binding)
		}
	}
	return unique
}

func devinTupleKey(binding devinAcceptedBinding) string {
	return binding.lead.FamilyUID + "\x00" + binding.sidekick.UID
}

// devinDimensionChoices preserves first occurrence order from the ACP group.
func devinDimensionChoices(
	accepted []devinAcceptedBinding,
) ([]modelcatalog.Choose, []modelcatalog.Choose) {
	leads := make([]modelcatalog.Choose, 0, len(accepted))
	sidekicks := make([]modelcatalog.Choose, 0, len(accepted))
	leadSeen := make(map[string]struct{}, len(accepted))
	sidekickSeen := make(map[string]struct{}, len(accepted))
	for _, binding := range accepted {
		if _, exists := leadSeen[binding.lead.FamilyUID]; !exists {
			leadSeen[binding.lead.FamilyUID] = struct{}{}
			leads = append(leads, modelcatalog.Choose{
				Value: binding.lead.FamilyUID,
				Label: binding.lead.FamilyLabel,
			})
		}
		if _, exists := sidekickSeen[binding.sidekick.UID]; !exists {
			sidekickSeen[binding.sidekick.UID] = struct{}{}
			sidekicks = append(sidekicks, modelcatalog.Choose{
				Value: binding.sidekick.UID,
				Label: binding.sidekick.Label,
			})
		}
	}
	return leads, sidekicks
}

func cloneDevinCatalog(catalog *modelcatalog.Catalog) *modelcatalog.Catalog {
	clone := &modelcatalog.Catalog{
		Version:      catalog.Version,
		Nodes:        cloneDevinNodes(catalog.Nodes),
		Compositions: append([]modelcatalog.Composition(nil), catalog.Compositions...),
	}
	for i := range clone.Compositions {
		clone.Compositions[i] = cloneDevinComposition(catalog.Compositions[i])
	}
	return clone
}

func cloneDevinNodes(nodes []modelcatalog.Node) []modelcatalog.Node {
	if nodes == nil {
		return nil
	}
	clone := make([]modelcatalog.Node, len(nodes))
	for i, node := range nodes {
		clone[i] = node
		clone[i].Children = cloneDevinNodes(node.Children)
	}
	return clone
}

func cloneDevinComposition(composition modelcatalog.Composition) modelcatalog.Composition {
	clone := composition
	clone.Dimensions = make([]modelcatalog.Dimension, len(composition.Dimensions))
	for i, dimension := range composition.Dimensions {
		clone.Dimensions[i] = dimension
		clone.Dimensions[i].Choices = append([]modelcatalog.Choose(nil), dimension.Choices...)
	}
	clone.Bindings = make([]modelcatalog.Binding, len(composition.Bindings))
	for i, binding := range composition.Bindings {
		clone.Bindings[i] = modelcatalog.Binding{Model: binding.Model, Values: make(map[string]string, len(binding.Values))}
		for key, value := range binding.Values {
			clone.Bindings[i].Values[key] = value
		}
	}
	return clone
}
