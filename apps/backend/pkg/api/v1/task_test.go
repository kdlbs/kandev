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

func TestAggregateForegroundActivity(t *testing.T) {
	gen := ForegroundActivityGenerating
	bg := ForegroundActivityBackground
	tests := []struct {
		name       string
		activities []ForegroundActivity
		want       ForegroundActivity
	}{
		{name: "empty is empty", activities: nil, want: ""},
		{name: "any generating wins over background", activities: []ForegroundActivity{bg, gen}, want: gen},
		{name: "all background is background", activities: []ForegroundActivity{bg, bg}, want: bg},
		{name: "single background", activities: []ForegroundActivity{bg}, want: bg},
		{name: "single generating", activities: []ForegroundActivity{gen}, want: gen},
		{name: "empty values are ignored", activities: []ForegroundActivity{"", bg, ""}, want: bg},
		{name: "only empty values fall through", activities: []ForegroundActivity{"", ""}, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AggregateForegroundActivity(tc.activities); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
