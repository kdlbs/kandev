package profiles

import "testing"

func TestAsyncQuestionsOffInEveryShippedProfile(t *testing.T) {
	for _, profile := range []struct {
		name     string
		selector map[string]string
	}{
		{name: "prod"},
		{name: "dev", selector: map[string]string{"KANDEV_DEBUG_DEV_MODE": "true"}},
		{name: "e2e", selector: map[string]string{"KANDEV_E2E_MOCK": "true"}},
	} {
		t.Run(profile.name, func(t *testing.T) {
			clearProfileSelectors(t)
			clearProfilesYAMLVars(t)
			for name, value := range profile.selector {
				t.Setenv(name, value)
			}

			defaults, err := EnvironmentDefaults()
			if err != nil {
				t.Fatalf("EnvironmentDefaults: %v", err)
			}
			if got := defaults["KANDEV_FEATURES_ASYNC_QUESTIONS"]; got != "false" {
				t.Fatalf("KANDEV_FEATURES_ASYNC_QUESTIONS = %q in %s, want false", got, profile.name)
			}
		})
	}
}
