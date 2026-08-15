package backendapp

import "testing"

func TestDefaultProviderHostnameRejectsPluginOwnedProviders(t *testing.T) {
	if _, err := defaultProviderHostname("bitbucket"); err == nil {
		t.Fatal("defaultProviderHostname(bitbucket) expected an error; plugin providers must supply their persisted clone URL")
	}
}
