package v1

import "testing"

func TestMessageAttachmentHasValidDeliveryMode(t *testing.T) {
	tests := []struct {
		name         string
		deliveryMode string
		want         bool
	}{
		{name: "empty defaults to prompt", deliveryMode: "", want: true},
		{name: "prompt", deliveryMode: "prompt", want: true},
		{name: "path", deliveryMode: "path", want: true},
		{name: "invalid", deliveryMode: "inline", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (MessageAttachment{DeliveryMode: tt.deliveryMode}).HasValidDeliveryMode()
			if got != tt.want {
				t.Fatalf("HasValidDeliveryMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
