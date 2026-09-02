package configsync

import "testing"

func TestDecideKey(t *testing.T) {
	cases := []struct {
		name                                                                   string
		inFetched, inManifest, manifestEntityExists, unmanagedHoldsKey, exempt bool
		want                                                                   applyDecision
	}{
		{"new: fetched only, no unmanaged collision", true, false, false, false, false, decisionNew},
		{"foreign: fetched only, unmanaged entity holds key", true, false, false, true, false, decisionForeign},
		{"existing: fetched and manifested, entity present", true, true, true, false, false, decisionExisting},
		{"new: fetched and manifested but manifest entity gone", true, true, false, false, false, decisionNew},
		{"foreign: fetched and manifested but manifest entity gone, unmanaged row holds key", true, true, false, true, false, decisionForeign},
		{"gone out of band: manifested only, entity gone", false, true, false, false, false, decisionGoneOutOfBand},
		{"exempt: manifested only, entity present, exempt", false, true, true, false, true, decisionExempt},
		{"removed upstream: manifested only, entity present, not exempt", false, true, true, false, false, decisionRemovedUpstream},
		{"skip: neither fetched nor manifested", false, false, false, false, false, decisionSkip},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideKey(tc.inFetched, tc.inManifest, tc.manifestEntityExists, tc.unmanagedHoldsKey, tc.exempt)
			if got != tc.want {
				t.Errorf("decideKey(%v,%v,%v,%v,%v) = %v, want %v",
					tc.inFetched, tc.inManifest, tc.manifestEntityExists, tc.unmanagedHoldsKey, tc.exempt, got, tc.want)
			}
		})
	}
}
