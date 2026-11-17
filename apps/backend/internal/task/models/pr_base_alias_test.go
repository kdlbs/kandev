package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPRBaseDoesNotSerializeLiveHeadAlias(t *testing.T) {
	encoded, err := json.Marshal(PRBase{HeadRepositoryNodeID: "live-node-alias"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "live-node-alias") {
		t.Fatalf("live alias was serialized: %s", encoded)
	}
}
