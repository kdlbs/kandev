package webapp

import "testing"

func TestPrefersHTML(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   bool
	}{
		{"empty header stays JSON", "", false},
		{"json preferred over html stays JSON", "application/json;q=1, text/html;q=0.1", false},
		{"equal quality stays JSON", "text/html;q=0.5, application/json;q=0.5", false},
		{"bare wildcard stays JSON", "*/*", false},
		{"html only prefers HTML", "text/html", true},
		{"typical browser navigation header prefers HTML", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", true},
		{"specific html wildcard loses to a higher-quality catch-all", "text/*;q=0.9,*/*;q=1.0", false},
		{"exact match beats type wildcard even at lower q", "text/html;q=0.6,text/*;q=0.9", true},
		{"malformed q value ignored", "text/html;q=bogus", false},
		{"malformed media type ignored", "not-a-media-type;q=1", false},
		{"json wildcard beats html when html unlisted", "application/*;q=1", false},
		{"zero quality html excluded", "text/html;q=0", false},
		{
			"explicit q=0 exclusion beats a higher-quality wildcard",
			"text/html;q=0.5, application/json;q=0, */*;q=1",
			true,
		},
		{"malformed q with trailing garbage ignored", "text/html;q=.5junk", false},
		{"out-of-range q ignored, does not leak wildcard quality", "text/html;q=2, */*;q=0.5", false},
		{"uppercase Q parameter name is honored", "text/html;Q=0.9", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrefersHTML(tc.header); got != tc.want {
				t.Fatalf("PrefersHTML(%q) = %v, want %v", tc.header, got, tc.want)
			}
		})
	}
}
