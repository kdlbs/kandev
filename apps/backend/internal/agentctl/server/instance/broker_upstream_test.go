package instance

import (
	"errors"
	"testing"
)

func TestValidateBrokerUpstream(t *testing.T) {
	valid := []string{"", "http://127.0.0.1:8080/api/v1/git/credentials/resolve", "https://kandev.example/x/resolve"}
	for _, raw := range valid {
		if err := ValidateBrokerUpstream(raw); err != nil {
			t.Errorf("%q rejected: %v", raw, err)
		}
	}
	invalid := []string{
		" http://h/resolve", "ftp://h/resolve", "/resolve", "http:///resolve",
		"http://u:p@h/resolve", "http://h/resolve?x=1", "http://h/resolve#f", "http://h/resolve?",
		"http://h/other", "http://h/resolve/", "://bad",
	}
	for _, raw := range invalid {
		if err := ValidateBrokerUpstream(raw); !errors.Is(err, ErrInvalidBrokerUpstream) {
			t.Errorf("%q err = %v, want ErrInvalidBrokerUpstream", raw, err)
		}
	}
}
