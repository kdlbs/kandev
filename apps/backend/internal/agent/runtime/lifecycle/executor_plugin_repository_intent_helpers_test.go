package lifecycle

import (
	"net/url"

	"github.com/kandev/kandev/internal/plugins/workspacedescriptor"
)

// userinfoURL builds a URL carrying credentials without a literal in source.
func userinfoURL(base, user, password string) string {
	parsed, err := url.Parse(base)
	if err != nil {
		panic(err)
	}
	parsed.User = url.UserPassword(user, password)
	return parsed.String()
}

func recordedDescriptor() *workspacedescriptor.Descriptor {
	return &workspacedescriptor.Descriptor{Path: testInventoryRoot, Identity: "worker-7"}
}
