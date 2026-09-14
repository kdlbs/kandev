package manifest

import (
	"strings"
	"testing"
)

func TestValidateCanvasDistribution(t *testing.T) {
	m := canvasDistributionManifest()
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if !m.IsCanvasDistribution() {
		t.Fatal("IsCanvasDistribution() = false, want true")
	}
	if m.Distribution.SourceMode != SourceModeProject {
		t.Fatalf("source mode = %q, want %q", m.Distribution.SourceMode, SourceModeProject)
	}
}

func TestValidateCanvasDistributionRejectsOtherContributions(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{name: "missing license", mutate: func(m *Manifest) { m.Distribution.License = "" }, want: "distribution.license"},
		{name: "missing compatibility", mutate: func(m *Manifest) { m.MinKandevVersion = "" }, want: "min_kandev_version"},
		{name: "managed runtime", mutate: func(m *Manifest) { m.Runtime.Type = "binary" }, want: "runtime.type"},
		{name: "native bundle", mutate: func(m *Manifest) { m.UI.Bundle = "/bundle.js" }, want: "native UI"},
		{name: "action", mutate: func(m *Manifest) {
			m.Actions = []Action{{Key: "run", ResourceScope: ActionScopeWorkspace, MaxBodyBytes: 1}}
		}, want: "backend"},
		{name: "task only", mutate: func(m *Manifest) { m.UI.WebApps[0].Placements = []string{WebAppPlacementTask} }, want: "workspace-canvas"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := canvasDistributionManifest()
			tc.mutate(m)
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want text %q", err, tc.want)
			}
		})
	}
}

func canvasDistributionManifest() *Manifest {
	return &Manifest{
		ID: "canvas-board", APIVersion: CurrentAPIVersion, Version: "1.2.3",
		DisplayName: "Canvas Board", Description: "A board", Author: "Kandev",
		Categories: []string{"canvas"}, MinKandevVersion: "1.0.0",
		Distribution: &Distribution{
			SchemaVersion: DistributionSchemaVersion, Kind: DistributionKindCanvas,
			License: "MIT", SourceMode: SourceModeProject,
		},
		UI: UISection{WebApps: []WebApp{{
			Key: "main", Title: "Board", Entry: "ui/index.html",
			Placements: []string{WebAppPlacementWorkspace},
		}}},
	}
}
