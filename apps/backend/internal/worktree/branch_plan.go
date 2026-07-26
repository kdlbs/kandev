package worktree

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
)

// BranchIdentityInput contains the durable branch fields used to plan a
// repository worktree's stable identity and path slug.
type BranchIdentityInput struct {
	RepositoryID   string
	BaseBranch     string
	CheckoutBranch string
	DefaultBranch  string
	PRNumber       int
	Position       int
}

// BranchIdentityPlan is the stable identity and optional path suffix for one
// BranchIdentityInput at the same index in BuildBranchIdentityPlans' result.
type BranchIdentityPlan struct {
	IdentitySlug string
	PathSlug     string
}

// BuildBranchIdentityPlans derives stable identities for repository branches.
// The lowest-ranked branch for a repeated repository keeps the legacy flat
// path; every plan still has an identity for durable worktree reuse.
func BuildBranchIdentityPlans(inputs []BranchIdentityInput) []BranchIdentityPlan {
	plans := make([]BranchIdentityPlan, len(inputs))
	groups := make(map[string][]int, len(inputs))
	for index, input := range inputs {
		if input.RepositoryID != "" {
			groups[input.RepositoryID] = append(groups[input.RepositoryID], index)
		}
	}
	for repositoryID, group := range groups {
		identities := branchIdentitySlugsForGroup(repositoryID, inputs, group)
		if len(group) == 1 {
			index := group[0]
			plans[index] = BranchIdentityPlan{IdentitySlug: identities[index]}
			continue
		}
		flatIdentity := selectFlatBranchIdentity(inputs, group, identities)
		for _, index := range group {
			pathSlug := identities[index]
			if pathSlug == flatIdentity {
				pathSlug = ""
			}
			plans[index] = BranchIdentityPlan{IdentitySlug: identities[index], PathSlug: pathSlug}
		}
	}
	return plans
}

func branchIdentitySlugsForGroup(repositoryID string, inputs []BranchIdentityInput, group []int) map[int]string {
	raw := make(map[int]string, len(group))
	counts := make(map[string]int, len(group))
	for _, index := range group {
		slug := preferredBranchIdentitySlug(inputs[index])
		if slug == "" {
			slug = "branch-" + branchIdentityHash(repositoryID, inputs[index])
		}
		raw[index] = slug
		counts[slug]++
	}
	identities := make(map[int]string, len(group))
	for _, index := range group {
		slug := raw[index]
		if counts[slug] > 1 {
			slug += "-" + branchIdentityHash(repositoryID, inputs[index])
		}
		slug = SanitizeBranchSlug(slug)
		if slug == "" {
			slug = "branch-" + branchIdentityHash(repositoryID, inputs[index])
		}
		identities[index] = slug
	}
	return identities
}

func preferredBranchIdentitySlug(input BranchIdentityInput) string {
	branch := input.CheckoutBranch
	if branch == "" {
		branch = effectiveBaseBranch(input)
	}
	return SanitizeBranchSlug(branch)
}

func branchIdentityHash(repositoryID string, input BranchIdentityInput) string {
	seed := strings.Join([]string{repositoryID, effectiveBaseBranch(input), input.CheckoutBranch, fmt.Sprintf("%d", input.PRNumber)}, "\x00")
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(seed))
	return fmt.Sprintf("%08x", hash.Sum32())
}

func effectiveBaseBranch(input BranchIdentityInput) string {
	if input.BaseBranch != "" {
		return input.BaseBranch
	}
	return input.DefaultBranch
}

func selectFlatBranchIdentity(inputs []BranchIdentityInput, group []int, identities map[int]string) string {
	candidates := append([]int(nil), group...)
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := inputs[candidates[i]], inputs[candidates[j]]
		if left.Position != right.Position {
			return left.Position < right.Position
		}
		if leftRank, rightRank := flatBranchRank(left), flatBranchRank(right); leftRank != rightRank {
			return leftRank < rightRank
		}
		return identities[candidates[i]] < identities[candidates[j]]
	})
	return identities[candidates[0]]
}

func flatBranchRank(input BranchIdentityInput) int {
	if input.CheckoutBranch != "" {
		return 3
	}
	baseBranch := effectiveBaseBranch(input)
	if input.DefaultBranch != "" && baseBranch == input.DefaultBranch {
		return 0
	}
	if baseBranch == "main" {
		return 1
	}
	return 2
}
