package automation

import "testing"

func TestEvaluateFilters_EmptyAlwaysPasses(t *testing.T) {
	idx, ok := EvaluateFilters(nil, map[string]interface{}{"a": 1})
	if !ok || idx != -1 {
		t.Fatalf("empty filter list: idx=%d ok=%v, want -1/true", idx, ok)
	}
}

func TestEvaluateFilters_ShortCircuitsAtFirstFailure(t *testing.T) {
	filters := []WebhookFilter{
		{Path: "severity", Op: WebhookFilterOpEq, Values: []string{"critical"}},
		{Path: "env", Op: WebhookFilterOpEq, Values: []string{"prod"}},
	}
	data := map[string]interface{}{"severity": "warning", "env": "prod"}
	idx, ok := EvaluateFilters(filters, data)
	if ok || idx != 0 {
		t.Fatalf("idx=%d ok=%v, want 0/false (first predicate should reject)", idx, ok)
	}
}

func TestEvaluateFilters_AllPassSucceeds(t *testing.T) {
	filters := []WebhookFilter{
		{Path: "severity", Op: WebhookFilterOpEq, Values: []string{"critical"}},
		{Path: "env", Op: WebhookFilterOpNe, Values: []string{"staging"}},
	}
	data := map[string]interface{}{"severity": "critical", "env": "prod"}
	idx, ok := EvaluateFilters(filters, data)
	if !ok || idx != -1 {
		t.Fatalf("idx=%d ok=%v, want -1/true", idx, ok)
	}
}

func TestEvaluateFilters_Operators(t *testing.T) {
	data := map[string]interface{}{
		"severity": "Critical",
		"tags":     "prod outage",
	}
	cases := []struct {
		name   string
		filter WebhookFilter
		want   bool
	}{
		{"eq case-insensitive trimmed", WebhookFilter{Path: "severity", Op: WebhookFilterOpEq, Values: []string{"  critical "}}, true},
		{"eq mismatch", WebhookFilter{Path: "severity", Op: WebhookFilterOpEq, Values: []string{"low"}}, false},
		{"ne mismatch passes", WebhookFilter{Path: "severity", Op: WebhookFilterOpNe, Values: []string{"low"}}, true},
		{"ne match fails", WebhookFilter{Path: "severity", Op: WebhookFilterOpNe, Values: []string{"critical"}}, false},
		{"ne missing path fails closed", WebhookFilter{Path: "missing", Op: WebhookFilterOpNe, Values: []string{"x"}}, false},
		{"in member", WebhookFilter{Path: "severity", Op: WebhookFilterOpIn, Values: []string{"low", "critical"}}, true},
		{"in non-member", WebhookFilter{Path: "severity", Op: WebhookFilterOpIn, Values: []string{"low"}}, false},
		{"not_in non-member passes", WebhookFilter{Path: "severity", Op: WebhookFilterOpNotIn, Values: []string{"low"}}, true},
		{"not_in member fails", WebhookFilter{Path: "severity", Op: WebhookFilterOpNotIn, Values: []string{"critical"}}, false},
		{"exists present", WebhookFilter{Path: "severity", Op: WebhookFilterOpExists}, true},
		{"exists absent", WebhookFilter{Path: "missing", Op: WebhookFilterOpExists}, false},
		{"exists rejects values", WebhookFilter{Path: "severity", Op: WebhookFilterOpExists, Values: []string{"x"}}, false},
		{"not_exists absent", WebhookFilter{Path: "missing", Op: WebhookFilterOpNotExists}, true},
		{"not_exists present", WebhookFilter{Path: "severity", Op: WebhookFilterOpNotExists}, false},
		{"contains substring", WebhookFilter{Path: "tags", Op: WebhookFilterOpContains, Values: []string{"outage"}}, true},
		{"contains miss", WebhookFilter{Path: "tags", Op: WebhookFilterOpContains, Values: []string{"resolved"}}, false},
		{"unknown op fails closed", WebhookFilter{Path: "severity", Op: "bogus"}, false},
		{"eq wrong arity fails closed", WebhookFilter{Path: "severity", Op: WebhookFilterOpEq, Values: []string{"a", "b"}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := EvaluateFilters([]WebhookFilter{tc.filter}, data)
			if ok != tc.want {
				t.Errorf("got ok=%v, want %v", ok, tc.want)
			}
		})
	}
}

func TestEvaluateFilters_NestedPath(t *testing.T) {
	data := map[string]interface{}{
		"alert": map[string]interface{}{"severity": "critical"},
	}
	filters := []WebhookFilter{{Path: "alert.severity", Op: WebhookFilterOpEq, Values: []string{"critical"}}}
	idx, ok := EvaluateFilters(filters, data)
	if !ok || idx != -1 {
		t.Fatalf("idx=%d ok=%v, want -1/true", idx, ok)
	}
}
