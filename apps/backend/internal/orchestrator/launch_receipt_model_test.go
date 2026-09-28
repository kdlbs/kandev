package orchestrator

import "testing"

func TestLaunchReceiptModelProvenanceIsOptional(t *testing.T) {
	unknown := LaunchReceipt{}
	if unknown.RequestedModel != "" || unknown.EffectiveModel != "" {
		t.Fatalf("unknown model provenance = %+v", unknown)
	}
	known := LaunchReceipt{RequestedModel: "requested", EffectiveModel: "effective"}
	if known.RequestedModel != "requested" || known.EffectiveModel != "effective" {
		t.Fatalf("known model provenance = %+v", known)
	}
}
