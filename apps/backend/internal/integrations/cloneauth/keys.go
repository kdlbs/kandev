package cloneauth

// AzureDevOpsPATKey returns the encrypted-secret key used for a workspace PAT.
func AzureDevOpsPATKey(workspaceID string) string {
	return "azure_devops:" + workspaceID + ":pat"
}
