package acpcompat

import (
	"github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/common/modelcatalog"
)

const (
	// DevinAgentID is the registered identity of Cognition's Devin CLI agent.
	DevinAgentID = "devin-acp"

	// DevinGroupedSessionConfigOptionsMetaKey opts the client into Devin's
	// grouped session config options extension. Native ACP groups are the
	// source of the shared catalog's group level.
	DevinGroupedSessionConfigOptionsMetaKey = "cognition.ai/groupedSessionConfigOptions"
)

// CatalogFromSelectOptions converts native ACP groups into the neutral
// catalog. Ungrouped options, empty groups, and data outside the catalog
// bounds return nil so callers keep the validated flat choice list.
func CatalogFromSelectOptions(options acp.SessionConfigSelectOptions) *modelcatalog.Catalog {
	if options.Grouped == nil || len(*options.Grouped) == 0 {
		return nil
	}
	groups := *options.Grouped
	if len(groups) > modelcatalog.MaxNodes {
		return nil
	}
	total := 0
	for _, group := range groups {
		total += len(group.Options)
		if total > modelcatalog.MaxNodes {
			return nil
		}
	}
	nodes := make([]modelcatalog.Node, 0, len(groups))
	for _, group := range groups {
		children := make([]modelcatalog.Node, 0, len(group.Options))
		for _, option := range group.Options {
			children = append(children, modelcatalog.Node{
				Kind:        modelcatalog.NodeKindChoice,
				Value:       string(option.Value),
				Label:       option.Name,
				Description: optionalDescription(option.Description),
			})
		}
		nodes = append(nodes, modelcatalog.Node{
			Kind:     modelcatalog.NodeKindGroup,
			ID:       string(group.Group),
			Label:    groupLabel(group),
			Children: children,
		})
	}
	return publishCatalog(nodes)
}

func groupLabel(group acp.SessionConfigSelectGroup) string {
	if group.Name != "" {
		return group.Name
	}
	return string(group.Group)
}

func optionalDescription(description *string) string {
	if description == nil {
		return ""
	}
	return *description
}

func publishCatalog(nodes []modelcatalog.Node) *modelcatalog.Catalog {
	catalog := &modelcatalog.Catalog{Version: modelcatalog.SchemaVersion, Nodes: nodes}
	if err := modelcatalog.Validate(catalog); err != nil {
		return nil
	}
	return catalog
}

// CatalogFromRawOptions converts the untyped `_meta.configOptions[].options`
// shape used by agents that bypass the SDK's typed parser. A flat list keeps
// its existing behavior; a grouped list publishes the same neutral catalog.
func CatalogFromRawOptions(raw any) *modelcatalog.Catalog {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	nodes := make([]modelcatalog.Node, 0, len(items))
	for _, item := range items {
		group, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		groupID, ok := group["group"].(string)
		if !ok || groupID == "" {
			return nil
		}
		children, ok := rawGroupChoices(group["options"])
		if !ok {
			return nil
		}
		nodes = append(nodes, modelcatalog.Node{
			Kind:     modelcatalog.NodeKindGroup,
			ID:       groupID,
			Label:    rawGroupLabel(group),
			Children: children,
		})
	}
	return publishCatalog(nodes)
}

func rawGroupLabel(group map[string]any) string {
	if name, ok := group["name"].(string); ok && name != "" {
		return name
	}
	label, _ := group["group"].(string)
	return label
}

func rawGroupChoices(raw any) ([]modelcatalog.Node, bool) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, false
	}
	children := make([]modelcatalog.Node, 0, len(items))
	for _, item := range items {
		choice, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := choice["value"].(string)
		if !ok || value == "" {
			return nil, false
		}
		name, _ := choice["name"].(string)
		description, _ := choice["description"].(string)
		children = append(children, modelcatalog.Node{
			Kind:        modelcatalog.NodeKindChoice,
			Value:       value,
			Label:       name,
			Description: description,
		})
	}
	return children, true
}
