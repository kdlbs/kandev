package runtimeflags

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrEnvLocked   = errors.New("runtime flag is controlled by environment")
	ErrStoreUnset  = errors.New("runtime flag store is not configured")
	ErrUnknownFlag = errors.New("unknown runtime flag")
)

type Options struct {
	DefaultValues map[string]bool
	RuntimeValues map[string]bool
	EnvValues     map[string]bool
	IsExplicitEnv func(name string) bool
}

type Service struct {
	store Store
	opts  Options
}

func NewService(store Store, opts Options) *Service {
	if opts.IsExplicitEnv == nil {
		opts.IsExplicitEnv = func(string) bool { return false }
	}
	return &Service{store: store, opts: opts}
}

func (s *Service) ListStates(ctx context.Context) ([]RuntimeFlagState, error) {
	if s.store == nil {
		return nil, ErrStoreUnset
	}
	overrides, err := s.store.ListOverrides(ctx)
	if err != nil {
		return nil, err
	}
	defs := Definitions()
	sort.Slice(defs, func(i, j int) bool { return defs[i].Key < defs[j].Key })
	states := make([]RuntimeFlagState, 0, len(defs))
	for _, def := range defs {
		states = append(states, s.stateFor(def, overrides))
	}
	return states, nil
}

func (s *Service) SetOverride(ctx context.Context, key string, value *bool) ([]RuntimeFlagState, error) {
	if s.store == nil {
		return nil, ErrStoreUnset
	}
	def, ok := DefinitionByKey(key)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownFlag, key)
	}
	if s.isEnvLocked(def) {
		return nil, ErrEnvLocked
	}
	if value == nil {
		if err := s.store.DeleteOverride(ctx, key); err != nil {
			return nil, err
		}
	} else if err := s.store.SetOverride(ctx, key, *value); err != nil {
		return nil, err
	}
	return s.ListStates(ctx)
}

func (s *Service) stateFor(def RuntimeFlagDefinition, overrides map[string]bool) RuntimeFlagState {
	defaultValue := s.opts.DefaultValues[def.Key]
	runtimeValue := s.opts.RuntimeValues[def.Key]
	effectiveValue := defaultValue
	source := SourceProfile
	var overrideValue *bool

	if value, ok := overrides[def.Key]; ok {
		v := value
		overrideValue = &v
		effectiveValue = value
		source = SourceOverride
	}
	if value, locked := s.envLockedValue(def, runtimeValue); locked {
		effectiveValue = value
		source = SourceEnv
	}
	if !defaultValue && source == SourceProfile {
		source = SourceDefault
	}

	return RuntimeFlagState{
		Key:                    def.Key,
		Kind:                   def.Kind,
		Label:                  def.Label,
		Description:            def.Description,
		Stability:              def.Stability,
		RiskLevel:              def.RiskLevel,
		RiskDescription:        def.RiskDescription,
		EffectiveValue:         effectiveValue,
		DefaultValue:           defaultValue,
		OverrideValue:          overrideValue,
		Source:                 source,
		EnvVar:                 def.EnvVar,
		EnvLocked:              source == SourceEnv,
		RestartRequired:        def.RestartRequired,
		RequiresRestartToApply: def.RestartRequired && effectiveValue != runtimeValue,
		Mutable:                def.Mutable,
	}
}

func (s *Service) isEnvLocked(def RuntimeFlagDefinition) bool {
	_, locked := s.envLockedValue(def, false)
	return locked
}

func (s *Service) envLockedValue(def RuntimeFlagDefinition, fallback bool) (bool, bool) {
	if s.opts.IsExplicitEnv(def.EnvVar) {
		return s.envValue(def.EnvVar, fallback), true
	}
	for _, name := range def.ImpliedEnvVars {
		if s.opts.IsExplicitEnv(name) {
			return s.envValue(name, fallback), true
		}
	}
	return fallback, false
}

func (s *Service) envValue(name string, fallback bool) bool {
	if value, ok := s.opts.EnvValues[name]; ok {
		return value
	}
	return fallback
}
