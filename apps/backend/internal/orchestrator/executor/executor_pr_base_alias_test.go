package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1
// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2
func TestPRBaseIdentityAcceptsOnlyVerifiedHeadAlias(t *testing.T) {
	for _, host := range []string{"github.com", "git.example.com:8443"} {
		t.Run(host, func(t *testing.T) {
			target := *forkPRComparisonTarget()
			target.HeadRepository.Host, target.TargetRepository.Host = host, host
			target.HeadRepository.ProviderID = "42"
			base := models.PRBase{Target: target, HeadRepositoryNodeID: "R_verified"}
			for _, tc := range []struct {
				name, id, host string
				want           bool
			}{
				{"numeric", "42", host, true},
				{"verified node", "R_verified", host, true},
				{"different node", "R_other", host, false},
				{"different numeric", "43", host, false},
				{"different host", "R_verified", "other.example.com", false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					head := target.HeadRepository
					head.ProviderID, head.Host = tc.id, tc.host
					expected := target
					expected.HeadRepository = head
					before := expected
					for _, bound := range []bool{false, true} {
						var stored *models.ComparisonTarget
						if bound {
							stored = &expected
						}
						valid, reason := validPRBaseIdentity(base, target.Number, target.HeadBranch, stored,
							target.TargetRepository, true, head, true, true)
						if valid != tc.want {
							t.Fatalf("bound=%v: valid=%v reason=%s, want %v", bound, valid, reason, tc.want)
						}
					}
					if expected != before {
						t.Fatal("stored comparison target was mutated")
					}
				})
			}
		})
	}
}

func TestPRBaseIdentityRequiresLiveAliasEvidence(t *testing.T) {
	target := *forkPRComparisonTarget()
	target.HeadRepository.ProviderID = "42"
	head := target.HeadRepository
	head.ProviderID = "R_verified"
	valid, _ := validPRBaseIdentity(models.PRBase{Target: target}, target.Number, target.HeadBranch,
		nil, target.TargetRepository, true, head, true, true)
	if valid {
		t.Fatal("accepted a node ID without live provider evidence")
	}
}
