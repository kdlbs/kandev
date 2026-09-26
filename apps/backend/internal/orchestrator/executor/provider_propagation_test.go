package executor

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeriveMCPProvidersReturnsCanonicalUnionFromAllRepositories(t *testing.T) {
	tests := []struct {
		name     string
		repos    []*repoInfo
		expected []string
	}{
		{
			name: "github only",
			repos: []*repoInfo{
				{Repository: &models.Repository{Provider: " GitHub "}},
			},
			expected: []string{"github"},
		},
		{
			name: "gitlab only",
			repos: []*repoInfo{
				{Repository: &models.Repository{Provider: "gitlab"}},
			},
			expected: []string{"gitlab"},
		},
		{
			name: "mixed order independent union",
			repos: []*repoInfo{
				{Repository: &models.Repository{Provider: "gitlab"}},
				{Repository: &models.Repository{Provider: "GitHub"}},
				{Repository: &models.Repository{Provider: "github"}},
			},
			expected: []string{"github", "gitlab"},
		},
		{
			name: "empty and unsupported",
			repos: []*repoInfo{
				{Repository: &models.Repository{Provider: "unsupported"}},
				{Repository: nil},
				nil,
			},
			expected: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := deriveMCPProviders(test.repos)
			if !reflect.DeepEqual(got, test.expected) {
				t.Fatalf("deriveMCPProviders() = %#v, want %#v", got, test.expected)
			}
		})
	}
}
