package pluginsdk

// SidebarGroupingProvider is host-owned metadata for a complete grouping. It
// contains no classification values, task IDs, or dispatchable action names.
type SidebarGroupingProvider struct {
	Reference  string `json:"reference"`
	Label      string `json:"label"`
	LabelKey   string `json:"labelKey,omitempty"`
	Generation string `json:"generation"`
}
