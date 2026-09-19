package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
)

type capturingConcreteDeployer struct {
	req skill.Request
}

func (d *capturingConcreteDeployer) Deploy(_ context.Context, req skill.Request) (skill.DeployResult, error) {
	d.req = req
	return skill.DeployResult{}, nil
}

func TestSkillDeployerAdapterPreservesOfficeRuntime(t *testing.T) {
	for _, tt := range []struct {
		name string
		want bool
	}{
		{name: "non-Office launch", want: false},
		{name: "Office launch", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inner := &capturingConcreteDeployer{}
			adapter := NewSkillDeployerAdapter(inner)

			_, err := adapter.DeploySkills(context.Background(), SkillDeployRequest{
				OfficeRuntime: tt.want,
			})
			if err != nil {
				t.Fatalf("DeploySkills: %v", err)
			}
			if inner.req.OfficeRuntime != tt.want {
				t.Errorf("OfficeRuntime = %t, want %t", inner.req.OfficeRuntime, tt.want)
			}
		})
	}
}
