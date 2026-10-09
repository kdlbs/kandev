package sessioncapacity

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestResolveSessionCapacity(t *testing.T) {
	saved := &Settings{Enabled: true, MaxSessions: 9}
	tests := []struct {
		name           string
		configured     *Settings
		environment    Environment
		wantSettings   Settings
		wantEffective  Effective
		wantInvalidEnv bool
	}{
		{
			name:          "absent saved state is disabled with remembered default",
			wantSettings:  Settings{Enabled: false, MaxSessions: DefaultMaxSessions},
			wantEffective: Effective{Enabled: false, MaxSessions: 0, Source: SourceDefault},
		},
		{
			name:          "saved disabled state remembers its maximum",
			configured:    &Settings{Enabled: false, MaxSessions: 11},
			wantSettings:  Settings{Enabled: false, MaxSessions: 11},
			wantEffective: Effective{Enabled: false, MaxSessions: 0, Source: SourceSetting},
		},
		{
			name:          "saved enabled state supplies effective capacity",
			configured:    saved,
			wantSettings:  *saved,
			wantEffective: Effective{Enabled: true, MaxSessions: 9, Source: SourceSetting},
		},
		{
			name:          "valid environment value wins and locks",
			configured:    saved,
			environment:   Environment{Value: " 7 ", Present: true},
			wantSettings:  *saved,
			wantEffective: Effective{Enabled: true, MaxSessions: 7, Source: SourceEnvironment, Locked: true},
		},
		{
			name:          "zero environment value disables and locks",
			configured:    saved,
			environment:   Environment{Value: "0", Present: true},
			wantSettings:  *saved,
			wantEffective: Effective{Enabled: false, MaxSessions: 0, Source: SourceEnvironment, Locked: true},
		},
		{
			name:           "blank environment falls back without warning",
			configured:     saved,
			environment:    Environment{Value: "  ", Present: true},
			wantSettings:   *saved,
			wantEffective:  Effective{Enabled: true, MaxSessions: 9, Source: SourceSetting},
			wantInvalidEnv: false,
		},
		{
			name:           "invalid environment falls back and is reported",
			configured:     saved,
			environment:    Environment{Value: "not-a-number", Present: true},
			wantSettings:   *saved,
			wantEffective:  Effective{Enabled: true, MaxSessions: 9, Source: SourceSetting},
			wantInvalidEnv: true,
		},
		{
			name:           "negative environment falls back and is reported",
			configured:     saved,
			environment:    Environment{Value: "-1", Present: true},
			wantSettings:   *saved,
			wantEffective:  Effective{Enabled: true, MaxSessions: 9, Source: SourceSetting},
			wantInvalidEnv: true,
		},
		{
			name:           "overflow environment falls back and is reported",
			configured:     saved,
			environment:    Environment{Value: "2147483648", Present: true},
			wantSettings:   *saved,
			wantEffective:  Effective{Enabled: true, MaxSessions: 9, Source: SourceSetting},
			wantInvalidEnv: true,
		},
		{
			name:          "maximum environment value is valid",
			environment:   Environment{Value: "2147483647", Present: true},
			wantSettings:  Settings{Enabled: false, MaxSessions: DefaultMaxSessions},
			wantEffective: Effective{Enabled: true, MaxSessions: 2147483647, Source: SourceEnvironment, Locked: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.configured, tt.environment)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if !reflect.DeepEqual(got.Settings, tt.wantSettings) {
				t.Fatalf("settings = %+v, want %+v", got.Settings, tt.wantSettings)
			}
			if !reflect.DeepEqual(got.Effective, tt.wantEffective) {
				t.Fatalf("effective = %+v, want %+v", got.Effective, tt.wantEffective)
			}
			if got.InvalidEnvironment != tt.wantInvalidEnv {
				t.Fatalf("invalid environment = %v, want %v", got.InvalidEnvironment, tt.wantInvalidEnv)
			}
		})
	}
}

func TestValidateSavedSettingsRequiresPortablePositiveMaximum(t *testing.T) {
	for _, tt := range []struct {
		name     string
		settings Settings
		wantErr  bool
	}{
		{name: "disabled zero", settings: Settings{MaxSessions: 0}, wantErr: true},
		{name: "negative", settings: Settings{MaxSessions: -1}, wantErr: true},
		{name: "overflow", settings: Settings{MaxSessions: 2147483648}, wantErr: true},
		{name: "minimum", settings: Settings{MaxSessions: 1}},
		{name: "maximum", settings: Settings{MaxSessions: 2147483647}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.settings)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, ErrValidation) {
				t.Fatalf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestSettingsAndPatchUseSnakeCaseAndPreserveOmittedFields(t *testing.T) {
	settingsJSON, err := json.Marshal(Settings{Enabled: true, MaxSessions: 8})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	if string(settingsJSON) != `{"enabled":true,"max_sessions":8}` {
		t.Fatalf("settings JSON = %s", settingsJSON)
	}

	var patch SettingsPatch
	if err := json.Unmarshal([]byte(`{"enabled":false}`), &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if patch.Enabled == nil || *patch.Enabled {
		t.Fatalf("enabled patch = %+v, want explicit false", patch.Enabled)
	}
	if patch.MaxSessions != nil {
		t.Fatalf("omitted max_sessions = %v, want nil", *patch.MaxSessions)
	}
	got := patch.Apply(Settings{Enabled: true, MaxSessions: 8})
	if !reflect.DeepEqual(got, Settings{Enabled: false, MaxSessions: 8}) {
		t.Fatalf("applied patch = %+v", got)
	}
}

func TestSettingsPatchRejectsNullAndWrongTypes(t *testing.T) {
	for _, input := range []string{
		`null`,
		`{"enabled":null}`,
		`{"max_sessions":null}`,
		`{"enabled":"true"}`,
		`{"max_sessions":"8"}`,
		`{"max_sessions":1.5}`,
		`{"unknown":true}`,
	} {
		t.Run(input, func(t *testing.T) {
			var patch SettingsPatch
			if err := json.Unmarshal([]byte(input), &patch); err == nil {
				t.Fatalf("unmarshal %s succeeded, want error", input)
			}
		})
	}
}
