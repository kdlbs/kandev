package models

import "testing"

func TestAllStatuses(t *testing.T) {
	statuses := AllStatuses()
	if len(statuses) != 7 {
		t.Fatalf("expected 7 statuses, got %d", len(statuses))
	}

	for i, s := range statuses {
		if s.Order != i {
			t.Errorf("status %q: expected order %d, got %d", s.ID, i, s.Order)
		}
		if s.ID == "" || s.Label == "" || s.Color == "" {
			t.Errorf("status at order %d has empty fields: %+v", i, s)
		}
	}
}

func TestAllPriorities(t *testing.T) {
	priorities := AllPriorities()
	if len(priorities) != 5 {
		t.Fatalf("expected 5 priorities, got %d", len(priorities))
	}

	for i, p := range priorities {
		if p.Order != i {
			t.Errorf("priority %q: expected order %d, got %d", p.ID, i, p.Order)
		}
		if p.ID == "" || p.Label == "" || p.Color == "" {
			t.Errorf("priority at order %d has empty fields: %+v", i, p)
		}
	}
}

func TestAllRoles(t *testing.T) {
	roles := AllRoles()
	if len(roles) != 4 {
		t.Fatalf("expected 4 roles, got %d", len(roles))
	}

	seen := make(map[string]bool)
	for _, r := range roles {
		if r.ID == "" || r.Label == "" || r.Color == "" {
			t.Errorf("role has empty fields: %+v", r)
		}
		if seen[r.ID] {
			t.Errorf("duplicate role ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestAllExecutorTypes(t *testing.T) {
	types := AllExecutorTypes()
	if len(types) < 3 {
		t.Fatalf("expected at least 3 executor types, got %d", len(types))
	}

	for _, et := range types {
		if et.ID == "" || et.Label == "" {
			t.Errorf("executor type has empty fields: %+v", et)
		}
	}
}

func TestAllSkillSourceTypes(t *testing.T) {
	types := AllSkillSourceTypes()
	if len(types) != 4 {
		t.Fatalf("expected 4 skill source types, got %d", len(types))
	}

	inlineFound := false
	for _, st := range types {
		if st.ID == "inline" {
			inlineFound = true
			if st.ReadOnly {
				t.Error("inline skill source should not be read-only")
			}
		}
		if st.ReadOnly && st.ReadOnlyReason == "" {
			t.Errorf("read-only skill source %q should have a reason", st.ID)
		}
	}
	if !inlineFound {
		t.Error("expected inline skill source type")
	}
}

func TestAllProjectStatuses(t *testing.T) {
	statuses := AllProjectStatuses()
	if len(statuses) != 4 {
		t.Fatalf("expected 4 project statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.ID == "" || s.Label == "" || s.Color == "" {
			t.Errorf("project status has empty fields: %+v", s)
		}
	}
}

func TestAllAgentStatuses(t *testing.T) {
	statuses := AllAgentStatuses()
	if len(statuses) != 5 {
		t.Fatalf("expected 5 agent statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.ID == "" || s.Label == "" || s.Color == "" {
			t.Errorf("agent status has empty fields: %+v", s)
		}
	}
}

func TestAllRoutineRunStatuses(t *testing.T) {
	statuses := AllRoutineRunStatuses()
	if len(statuses) != 7 {
		t.Fatalf("expected 7 routine run statuses, got %d", len(statuses))
	}

	for _, s := range statuses {
		if s.ID == "" || s.Label == "" || s.Color == "" {
			t.Errorf("routine run status has empty fields: %+v", s)
		}
	}
}

func TestAllInboxItemTypes(t *testing.T) {
	types := AllInboxItemTypes()
	if len(types) != 4 {
		t.Fatalf("expected 4 inbox item types, got %d", len(types))
	}

	for _, it := range types {
		if it.ID == "" || it.Label == "" || it.Icon == "" {
			t.Errorf("inbox item type has empty fields: %+v", it)
		}
	}
}
