package managedruntime

// ManagedNPMCommand is the shared launch recipe for managed package commands.
func ManagedNPMCommand(packageSpec string, preferOnline bool) []string {
	preference := managedNPMOfflinePreference
	if preferOnline {
		preference = managedNPMOnlinePreference
	}
	args := append([]string{managedNPMCommandName, managedNPMYesFlag, preference}, NPMProjectPrefixArgs()...)
	return append(args, packageSpec)
}
