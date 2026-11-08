package sshlaunch

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/processidentity"
)

func testOwner() Owner {
	return Owner{
		TaskID:               "task-1",
		SessionID:            "session-1",
		ControllerInstanceID: "instance-1",
		TaskDir:              "/home/kandev/.kandev/tasks/task-1",
		SessionDir:           "/home/kandev/.kandev/tasks/task-1/sessions/session-1",
		Target: TargetBinding{
			Host:            "runner.example.test",
			Port:            "22",
			User:            "kandev",
			HostFingerprint: "SHA256:abc",
		},
	}
}

func testReceipt(t *testing.T) Receipt {
	t.Helper()
	generation, err := NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	return Receipt{
		Version:    Version,
		Generation: generation,
		Owner:      testOwner(),
		UID:        1000,
		Process: processidentity.Identity{
			PID:        4242,
			GroupID:    4242,
			SessionID:  4242,
			BirthToken: "linux:11111111-2222-3333-4444-555555555555:987654",
		},
	}
}

func TestReceiptValidate(t *testing.T) {
	if err := testReceipt(t).Validate(); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}

	cases := map[string]func(*Receipt){
		"version":       func(r *Receipt) { r.Version = 2 },
		"generation":    func(r *Receipt) { r.Generation = "short" },
		"task":          func(r *Receipt) { r.Owner.TaskID = "" },
		"session dir":   func(r *Receipt) { r.Owner.SessionDir = "" },
		"target user":   func(r *Receipt) { r.Owner.Target.User = "" },
		"process pid":   func(r *Receipt) { r.Process.PID = 0 },
		"process token": func(r *Receipt) { r.Process.BirthToken = "" },
		"negative uid":  func(r *Receipt) { r.UID = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			receipt := testReceipt(t)
			mutate(&receipt)
			if err := receipt.Validate(); !errors.Is(err, ErrIncompleteReceipt) {
				t.Fatalf("Validate error = %v, want ErrIncompleteReceipt", err)
			}
		})
	}
}

func TestReceiptValidateForRejectsDifferentOwner(t *testing.T) {
	receipt := testReceipt(t)
	other := testOwner()
	other.ControllerInstanceID = "instance-2"
	if err := receipt.ValidateFor(other); !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("ValidateFor error = %v, want ErrReceiptMismatch", err)
	}
	if err := receipt.ValidateFor(testOwner()); err != nil {
		t.Fatalf("ValidateFor matching owner: %v", err)
	}

	other = testOwner()
	other.Target.HostFingerprint = "SHA256:def"
	if err := receipt.ValidateFor(other); !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("ValidateFor error = %v, want ErrReceiptMismatch for changed fingerprint", err)
	}
}

func TestReceiptValidateBindingChecksGeneration(t *testing.T) {
	receipt := testReceipt(t)
	binding := Binding{Generation: receipt.Generation, Owner: receipt.Owner}
	if err := receipt.ValidateBinding(binding); err != nil {
		t.Fatalf("ValidateBinding: %v", err)
	}
	other, err := NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	binding.Generation = other
	if err := receipt.ValidateBinding(binding); !errors.Is(err, ErrReceiptMismatch) {
		t.Fatalf("ValidateBinding error = %v, want ErrReceiptMismatch", err)
	}
}

func TestReceiptMetadataRoundTripPreservesIdentity(t *testing.T) {
	receipt := testReceipt(t)
	metadata, err := receipt.Metadata()
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if metadata["process"].(map[string]interface{})["birth_token"] != receipt.Process.BirthToken {
		t.Fatalf("birth token changed across the metadata projection: %#v", metadata["process"])
	}
	recovered, ok := ReceiptFromMetadata(map[string]interface{}{MetadataKey: metadata})
	if !ok {
		t.Fatal("ReceiptFromMetadata did not recover the receipt")
	}
	if !reflect.DeepEqual(recovered, receipt) {
		t.Fatalf("recovered receipt = %#v, want %#v", recovered, receipt)
	}

	if _, ok := ReceiptFromMetadata(nil); ok {
		t.Fatal("nil metadata produced a receipt")
	}
	if _, ok := ReceiptFromMetadata(map[string]interface{}{MetadataKey: map[string]interface{}{"version": 1}}); ok {
		t.Fatal("incomplete metadata produced a receipt")
	}
	if _, ok := ReceiptFromMetadata(map[string]interface{}{MetadataKey: "not-a-receipt"}); ok {
		t.Fatal("malformed metadata produced a receipt")
	}
}

func TestNewGenerationIsUniqueAndWellFormed(t *testing.T) {
	first, err := NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	second, err := NewGeneration()
	if err != nil {
		t.Fatalf("NewGeneration: %v", err)
	}
	if first == second {
		t.Fatal("two generations are identical")
	}
	if !isGeneration(first) || !isGeneration(second) {
		t.Fatalf("generation shape = %q / %q", first, second)
	}
}
