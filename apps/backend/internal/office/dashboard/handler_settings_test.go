package dashboard

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
)

func TestPermissionMetadataMatchesKnownPermissionKeys(t *testing.T) {
	metadata := allPermissionMeta()
	keys := make([]string, 0, len(metadata))
	for _, permission := range metadata {
		keys = append(keys, permission.Key)
	}

	want := shared.AllPermissionKeys()
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("permission metadata keys = %v, want %v", keys, want)
	}
}
