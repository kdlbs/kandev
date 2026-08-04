package service

import (
	"errors"
	"testing"
)

func TestValidateAttachmentSizeAcceptsExact100MiB(t *testing.T) {
	if err := ValidateAttachmentSize(MaxAttachmentBytes); err != nil {
		t.Fatalf("exact maximum should be accepted: %v", err)
	}
}

func TestValidateAttachmentSizeRejectsOneByteOver100MiB(t *testing.T) {
	if err := ValidateAttachmentSize(MaxAttachmentBytes + 1); !errors.Is(err, ErrAttachmentTooLarge) {
		t.Fatalf("expected ErrAttachmentTooLarge, got %v", err)
	}
}

func TestValidateAttachmentBatchEnforcesCountAndAggregate(t *testing.T) {
	if err := ValidateAttachmentBatch([]int64{MaxAttachmentBytes / 2, MaxAttachmentBytes / 2}); err != nil {
		t.Fatalf("two fitting files should be accepted: %v", err)
	}
	if err := ValidateAttachmentBatch([]int64{MaxAttachmentBytes / 2, MaxAttachmentBytes/2 + 1}); !errors.Is(err, ErrAttachmentTotalTooLarge) {
		t.Fatalf("expected aggregate limit error, got %v", err)
	}
	tooMany := make([]int64, MaxAttachmentCount+1)
	for i := range tooMany {
		tooMany[i] = 1
	}
	if err := ValidateAttachmentBatch(tooMany); !errors.Is(err, ErrTooManyAttachments) {
		t.Fatalf("expected attachment count error, got %v", err)
	}
}
