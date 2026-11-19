package models

// TaskRepositoryInheritedCheckoutReuseOnly reports whether metadata contains
// the server-authored inherited checkout selector marker.
func TaskRepositoryInheritedCheckoutReuseOnly(metadata map[string]interface{}) bool {
	value, _ := metadata[TaskRepositoryInheritedCheckoutReuseOnlyMetadataKey].(bool)
	return value
}
