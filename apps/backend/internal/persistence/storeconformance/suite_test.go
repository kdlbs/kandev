package storeconformance

import (
	"testing"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func TestStoreCatalogCompleteness(t *testing.T) {
	if err := testconformance.ValidateAdapters(requiredstores.Catalog(), Adapters()); err != nil {
		t.Fatalf("store catalog completeness: %v", err)
	}
}

func TestStoreConformance(t *testing.T) {
	testconformance.Run(t, requiredstores.Catalog(), Adapters(), testconformance.RunOptions{})
}
