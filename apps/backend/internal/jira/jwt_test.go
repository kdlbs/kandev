package jira

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func makeJWT(t *testing.T, payload map[string]interface{}) string {
	t.Helper()
	header := `{"alg":"none","typ":"JWT"}`
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	enc := func(s []byte) string { return base64.RawURLEncoding.EncodeToString(s) }
	return enc([]byte(header)) + "." + enc(body) + ".sig"
}

func TestParseSessionCookieExpiry(t *testing.T) {
	future := time.Now().Add(30 * 24 * time.Hour).Unix()
	jwt := makeJWT(t, map[string]interface{}{"exp": future, "sub": "user"})

	cases := []struct {
		name  string
		input string
		want  int64
	}{
		{"jwt", jwt, future},
		{"not_a_jwt", "opaque-value", 0},
		{"jwt_without_exp", makeJWT(t, map[string]interface{}{"sub": "user"}), 0},
		{"empty", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSessionCookieExpiry(tc.input)
			if tc.want == 0 {
				if got != nil {
					t.Fatalf("want nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want time=%d, got nil", tc.want)
			}
			if got.Unix() != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got.Unix())
			}
		})
	}
}
