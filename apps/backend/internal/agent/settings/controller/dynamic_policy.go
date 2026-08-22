package controller

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

const (
	dynamicPolicyVersion int64 = 1

	dynamicPolicyOutcomeSkip = "skip"
	dynamicPolicyOutcomeStop = "stop"

	dynamicPolicyMaxRetries             int64 = 10
	dynamicPolicyMaxInitialIntervalSecs int64 = 3600
	dynamicPolicyMaxWaitSecs            int64 = 7 * 24 * 60 * 60
)

func defaultDynamicPolicyDocument() dto.DynamicAgentPolicyDTO {
	return dto.DynamicAgentPolicyDTO{
		Version:   dynamicPolicyVersion,
		Transient: defaultDynamicErrorPolicy(),
		Hard:      defaultDynamicErrorPolicy(),
	}
}

func defaultDynamicErrorPolicy() dto.DynamicErrorPolicyDTO {
	return dto.DynamicErrorPolicyDTO{OnExhausted: dynamicPolicyOutcomeSkip}
}

func legacyDynamicErrorPolicy(action string) dto.DynamicErrorPolicyDTO {
	policy := defaultDynamicErrorPolicy()
	switch action {
	case dynamicRouteActionRetrySame:
		policy.Retry = dto.DynamicRetryPolicyDTO{
			Enabled:                true,
			MaxRetries:             1,
			InitialIntervalSeconds: 5,
		}
		policy.OnExhausted = dynamicPolicyOutcomeStop
	case dynamicRouteActionStop:
		policy.OnExhausted = dynamicPolicyOutcomeStop
	}
	return policy
}

func normalizeDynamicCandidatePolicy(candidate *dto.DynamicAgentCandidateDTO) error {
	if candidate.Policies != nil {
		return normalizeCanonicalDynamicPolicy(candidate)
	}

	policy, err := normalizeLegacyDynamicPolicy(candidate)
	if err != nil {
		return err
	}
	if err := validateDynamicPolicyDocument(policy, candidate.Position); err != nil {
		return err
	}
	candidate.Policies = &policy
	candidate.Rules = nil
	return nil
}

func normalizeCanonicalDynamicPolicy(candidate *dto.DynamicAgentCandidateDTO) error {
	if len(candidate.Rules) > 0 {
		return fmt.Errorf("%w: candidates[%d].policies cannot be combined with legacy rules", ErrDynamicProfileRule, candidate.Position)
	}
	policy := *candidate.Policies
	if err := validateDynamicPolicyDocument(policy, candidate.Position); err != nil {
		return err
	}
	candidate.Policies = &policy
	candidate.Rules = nil
	return nil
}

func normalizeLegacyDynamicPolicy(candidate *dto.DynamicAgentCandidateDTO) (dto.DynamicAgentPolicyDTO, error) {
	policy := defaultDynamicPolicyDocument()
	classActions := map[routingerr.Class]string{}
	keys := make([]string, 0, len(candidate.Rules))
	for key := range candidate.Rules {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		if err := applyLegacyDynamicRule(&policy, classActions, candidate.Position, rawKey, candidate.Rules[rawKey]); err != nil {
			return dto.DynamicAgentPolicyDTO{}, err
		}
	}
	return policy, nil
}

func applyLegacyDynamicRule(
	policy *dto.DynamicAgentPolicyDTO,
	classActions map[routingerr.Class]string,
	position int,
	rawKey string,
	rawAction string,
) error {
	key := strings.TrimSpace(rawKey)
	action := strings.TrimSpace(rawAction)
	if key == "" {
		return fmt.Errorf("%w: candidates[%d].rules has an empty key", ErrDynamicProfileRule, position)
	}
	if !isDynamicRouteAction(action) {
		return fmt.Errorf("%w: candidates[%d].rules.%s=%s", ErrDynamicProfileRule, position, key, action)
	}
	if key == "on_provider_error" {
		legacyPolicy := legacyDynamicErrorPolicy(action)
		policy.Transient = legacyPolicy
		policy.Hard = legacyPolicy
		return nil
	}
	class := routingerr.ClassForCode(routingerr.Code(key))
	if class != routingerr.ClassTransient && class != routingerr.ClassHard {
		return fmt.Errorf("%w: candidates[%d].rules.%s is not a provider error code", ErrDynamicProfileRule, position, key)
	}
	if previous, ok := classActions[class]; ok && previous != action {
		return fmt.Errorf("%w: candidates[%d].rules has conflicting actions for %s errors", ErrDynamicProfileRule, position, class)
	}
	classActions[class] = action
	legacyPolicy := legacyDynamicErrorPolicy(action)
	if class == routingerr.ClassTransient {
		policy.Transient = legacyPolicy
	} else {
		policy.Hard = legacyPolicy
	}
	return nil
}

func validateDynamicPolicyDocument(policy dto.DynamicAgentPolicyDTO, position int) error {
	if policy.Version != dynamicPolicyVersion {
		return fmt.Errorf("%w: candidates[%d].policies.version=%d, supported version is %d", ErrDynamicProfileRule, position, policy.Version, dynamicPolicyVersion)
	}
	if err := validateDynamicErrorPolicy(policy.Transient, position, "transient"); err != nil {
		return err
	}
	if err := validateDynamicErrorPolicy(policy.Hard, position, "hard"); err != nil {
		return err
	}
	return nil
}

func validateDynamicErrorPolicy(policy dto.DynamicErrorPolicyDTO, position int, class string) error {
	field := fmt.Sprintf("candidates[%d].policies.%s", position, class)
	if policy.OnExhausted != dynamicPolicyOutcomeSkip && policy.OnExhausted != dynamicPolicyOutcomeStop {
		return fmt.Errorf("%w: %s.on_exhausted=%q must be skip or stop", ErrDynamicProfileRule, field, policy.OnExhausted)
	}
	if policy.Retry.Enabled {
		if policy.Retry.MaxRetries < 1 || policy.Retry.MaxRetries > dynamicPolicyMaxRetries {
			return fmt.Errorf("%w: %s.retry.max_retries must be between 1 and %d", ErrDynamicProfileRule, field, dynamicPolicyMaxRetries)
		}
		if policy.Retry.InitialIntervalSeconds < 1 || policy.Retry.InitialIntervalSeconds > dynamicPolicyMaxInitialIntervalSecs {
			return fmt.Errorf("%w: %s.retry.initial_interval_seconds must be between 1 and %d", ErrDynamicProfileRule, field, dynamicPolicyMaxInitialIntervalSecs)
		}
	} else if policy.Retry.MaxRetries != 0 || policy.Retry.InitialIntervalSeconds != 0 {
		return fmt.Errorf("%w: %s.retry disabled values must be zero", ErrDynamicProfileRule, field)
	}
	if policy.WaitForReset.Enabled {
		if policy.WaitForReset.MaxWaitSeconds < 1 || policy.WaitForReset.MaxWaitSeconds > dynamicPolicyMaxWaitSecs {
			return fmt.Errorf("%w: %s.wait_for_reset.max_wait_seconds must be between 1 and %d", ErrDynamicProfileRule, field, dynamicPolicyMaxWaitSecs)
		}
	} else if policy.WaitForReset.MaxWaitSeconds != 0 {
		return fmt.Errorf("%w: %s.wait_for_reset disabled value must be zero", ErrDynamicProfileRule, field)
	}
	return nil
}

func isDynamicRouteAction(action string) bool {
	switch action {
	case dynamicRouteActionRetrySame, dynamicRouteActionTryNext, dynamicRouteActionStop:
		return true
	default:
		return false
	}
}

func decodeDynamicPolicyDocument(raw string, position int) (dto.DynamicAgentPolicyDTO, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultDynamicPolicyDocument(), nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return dto.DynamicAgentPolicyDTO{}, fmt.Errorf("decode dynamic route policy: %w", err)
	}
	if _, hasVersion := fields["version"]; hasVersion {
		var policy dto.DynamicAgentPolicyDTO
		if err := json.Unmarshal([]byte(raw), &policy); err != nil {
			return dto.DynamicAgentPolicyDTO{}, fmt.Errorf("decode dynamic route policy: %w", err)
		}
		if err := validateDynamicPolicyDocument(policy, position); err != nil {
			return dto.DynamicAgentPolicyDTO{}, err
		}
		return policy, nil
	}

	var rules map[string]string
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return dto.DynamicAgentPolicyDTO{}, fmt.Errorf("decode legacy dynamic route rules: %w", err)
	}
	candidate := dto.DynamicAgentCandidateDTO{Position: position, Rules: rules}
	if err := normalizeDynamicCandidatePolicy(&candidate); err != nil {
		return dto.DynamicAgentPolicyDTO{}, err
	}
	return *candidate.Policies, nil
}
