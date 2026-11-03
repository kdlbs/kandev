package managedruntime

// ManagedNPMCommand is the shared launch recipe for managed package commands.
func ManagedNPMCommand(packageSpec string, preferOnline bool) []string {
	preference := "--prefer-offline"
	if preferOnline {
		preference = "--prefer-online"
	}
	args := append([]string{"npx", "--yes", preference}, NPMProjectPrefixArgs()...)
	return append(args, packageSpec)
}
