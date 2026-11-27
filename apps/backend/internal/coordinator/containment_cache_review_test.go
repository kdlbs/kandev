package coordinator

import (
	"fmt"
	"testing"
)

func TestContainmentDiagnosticHistoryIsBoundedAndKeepsExistingChanges(t *testing.T) {
	state := newAdmissionState()
	key := admissionKey{contained: true}
	for i := 0; i < 1001; i++ {
		if state.changed(fmt.Sprint(i), key) {
			t.Fatal("first observation reported a change")
		}
	}
	if len(state.last) > 1000 {
		t.Fatalf("diagnostic history retained %d coordinator identities", len(state.last))
	}
	if state.changed("0", admissionKey{}) {
		t.Fatal("evicted history reported a change")
	}
	if !state.changed("1000", admissionKey{}) {
		t.Fatal("retained history lost its change")
	}
}
