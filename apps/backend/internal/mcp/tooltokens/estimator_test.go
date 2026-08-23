package tooltokens

import "testing"

func TestKnownO200kVectors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want int
	}{
		{text: "", want: 0},
		{text: "hello", want: 1},
		{text: "hello world", want: 2},
		{text: "antidisestablishmentarianism", want: 6},
		{text: "お誕生日おめでとう", want: 8},
	}
	for _, test := range tests {
		test := test
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()
			got, err := EstimateToolJSON([]byte(test.text))
			if err != nil {
				t.Fatalf("EstimateToolJSON() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("EstimateToolJSON() = %d, want %d", got, test.want)
			}
		})
	}
}
