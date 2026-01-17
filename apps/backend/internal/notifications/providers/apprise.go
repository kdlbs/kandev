package providers

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type AppriseProvider struct{}

func NewAppriseProvider() *AppriseProvider {
	return &AppriseProvider{}
}

func (p *AppriseProvider) Available() bool {
	_, err := exec.LookPath("apprise")
	return err == nil
}

func (p *AppriseProvider) Validate(config map[string]interface{}) error {
	_, err := parseAppriseURLs(config)
	return err
}

func (p *AppriseProvider) Send(ctx context.Context, message Message) error {
	if !p.Available() {
		return fmt.Errorf("apprise not installed")
	}
	urls, err := parseAppriseURLs(message.Config)
	if err != nil {
		return err
	}
	if len(urls) == 0 {
		return fmt.Errorf("apprise urls not configured")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{"-t", message.Title, "-b", message.Body}
	args = append(args, urls...)
	cmd := exec.CommandContext(timeoutCtx, "apprise", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apprise failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func parseAppriseURLs(config map[string]interface{}) ([]string, error) {
	if config == nil {
		return nil, fmt.Errorf("apprise config missing")
	}
	raw, ok := config["urls"]
	if !ok {
		return nil, fmt.Errorf("apprise urls missing")
	}
	switch value := raw.(type) {
	case []string:
		return value, nil
	case []interface{}:
		urls := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if ok && strings.TrimSpace(text) != "" {
				urls = append(urls, strings.TrimSpace(text))
			}
		}
		return urls, nil
	case string:
		parts := strings.Split(value, "\n")
		var urls []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				urls = append(urls, part)
			}
		}
		return urls, nil
	default:
		return nil, fmt.Errorf("apprise urls must be a list of strings")
	}
}
