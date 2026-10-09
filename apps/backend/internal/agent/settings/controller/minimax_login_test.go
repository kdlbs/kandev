package controller

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestMiniMaxDiscoveryPublishesLoginVariants(t *testing.T) {
	raw, err := json.Marshal(buildLoginCommandDTO(agents.NewMiniMaxACP()))
	if err != nil {
		t.Fatal(err)
	}
	var command struct {
		Variants map[string][]string `json:"variants"`
	}
	if err := json.Unmarshal(raw, &command); err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"cn", "global"} {
		argv := command.Variants[region]
		if len(argv) < 2 || argv[len(argv)-2] != "--region" || argv[len(argv)-1] != region {
			t.Fatalf("%s variant missing from discovery: %s", region, raw)
		}
	}
}
