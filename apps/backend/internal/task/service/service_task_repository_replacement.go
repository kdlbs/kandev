package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/kandev/kandev/internal/task/models"
)

type preparedRepositoryReplacement struct {
	entries                     []preparedRepositoryReplacementEntry
	preserveCheckout            bool
	checkoutMutabilityAvailable bool
}

type preparedRepositoryReplacementEntry struct {
	input            TaskRepositoryInput
	repositoryID     string
	baseBranch       string
	label            string
	policy           *models.RepositoryBranchPolicy
	policyErr        error
	checkoutErr      error
	checkoutInputErr error
}

func cloneTaskRepositoryInputs(inputs []TaskRepositoryInput) []TaskRepositoryInput {
	if inputs == nil {
		return nil
	}
	result := append([]TaskRepositoryInput{}, inputs...)
	for i := range result {
		input := &result[i]
		if input.CheckoutOptions != nil {
			copy := *input.CheckoutOptions
			copy.SparseDirectories = append([]string(nil), copy.SparseDirectories...)
			input.CheckoutOptions = &copy
		}
		input.BranchPolicySnapshot = cloneBranchPolicy(input.BranchPolicySnapshot)
		if input.RemoteContribution != nil {
			copy := *input.RemoteContribution
			input.RemoteContribution = &copy
		}
		if input.ContributionDestination != nil {
			copy := *input.ContributionDestination
			if copy.CredentialBinding != nil {
				binding := *copy.CredentialBinding
				copy.CredentialBinding = &binding
			}
			input.ContributionDestination = &copy
		}
	}
	return result
}

// normalizeReplacementInputs keeps definitive input and provider failures ahead
// of UpdateTask's separate task-field commit, without inheriting unlocked rows.
func (s *Service) normalizeReplacementInputs(ctx context.Context, workspaceID string, inputs []TaskRepositoryInput) error {
	for i := range inputs {
		if err := s.validateRepositoryCheckoutInput(ctx, workspaceID, inputs[i]); err != nil {
			return err
		}
		options, err := models.NormalizeRepositoryCheckoutOptions(inputs[i].CheckoutOptions)
		if err != nil {
			return err
		}
		inputs[i].CheckoutOptions = options
	}
	return s.preflightRepositoryInputs(ctx, workspaceID, inputs)
}

func (s *Service) prepareRepositoryReplacement(ctx context.Context, workspaceID string, inputs []TaskRepositoryInput, task *models.Task) (*preparedRepositoryReplacement, error) {
	inputs = cloneTaskRepositoryInputs(inputs)
	if err := s.normalizeReplacementInputs(ctx, workspaceID, inputs); err != nil {
		return nil, err
	}
	byPath, err := s.repositoriesByLocalPath(ctx, workspaceID, inputs)
	if err != nil {
		return nil, err
	}
	prepared := &preparedRepositoryReplacement{
		preserveCheckout:            task != nil,
		checkoutMutabilityAvailable: s.taskEnvironments != nil,
	}
	for i, input := range inputs {
		entry, err := s.prepareReplacementEntry(ctx, workspaceID, i, input, byPath, task)
		if err != nil {
			return nil, err
		}
		prepared.entries = append(prepared.entries, entry)
	}

	return prepared, nil
}

func (s *Service) prepareReplacementEntry(ctx context.Context, workspaceID string, index int, input TaskRepositoryInput, byPath map[string]*models.Repository, task *models.Task) (preparedRepositoryReplacementEntry, error) {

	input, err := normalizeContributionBindings(input)
	if err != nil {
		return preparedRepositoryReplacementEntry{}, err
	}
	if input.BranchPolicyID != "" && input.RepositoryID == "" {
		return preparedRepositoryReplacementEntry{}, fmt.Errorf("%w: branch policy selection requires a repository id", ErrInvalidRepositoryBranchPolicy)
	}
	id, base, _, resolveErr := s.resolveRepoInput(ctx, workspaceID, input, byPath)
	if resolveErr != nil {
		if input.RepositoryID != "" {
			resolveErr = classifyRepositoryResolutionError(index, input.RepositoryID, resolveErr)
		}
		return preparedRepositoryReplacementEntry{}, resolveErr
	}
	if id == "" {
		return preparedRepositoryReplacementEntry{}, errors.New("repository_id is required")
	}
	entry := preparedRepositoryReplacementEntry{input: input, repositoryID: id, baseBranch: base,
		label: s.repoDisplayLabel(ctx, input, id)}
	// A canonical immutable snapshot can supersede a deleted or edited policy.
	entry.policy, entry.policyErr = s.resolveTaskRepositoryPolicy(ctx, id, input)
	if task != nil && input.CheckoutOptions == nil {
		check := input
		check.CheckoutOptions = &models.RepositoryCheckoutOptions{Version: 1, DownloadMode: models.DownloadStandard}
		entry.checkoutInputErr = s.validateRepositoryCheckoutInput(ctx, workspaceID, check)
	}
	if task != nil && input.CheckoutOptions != nil {
		entry.checkoutErr = s.validateTaskCheckoutCapabilities(ctx, &CreateTaskRequest{
			WorkspaceID: workspaceID, Metadata: task.Metadata, Repositories: []TaskRepositoryInput{input},
		})
	}
	return entry, nil
}

// finalize performs only pure rules over prepared inputs and the locked state.
func (p *preparedRepositoryReplacement) finalize(snapshot models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error) {
	rows := make([]*models.TaskRepository, 0, len(p.entries))
	seen := make(map[string]bool, len(p.entries))
	for i, entry := range p.entries {
		row, err := p.finalizeRow(i, entry, snapshot)
		if err != nil {
			return nil, err
		}
		if err := claimPreparedRepositorySlot(seen, row, entry.label); err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}

	return rows, nil
}

func (p *preparedRepositoryReplacement) finalizeRow(index int, entry preparedRepositoryReplacementEntry, snapshot models.TaskRepositoryReplacementSnapshot) (*models.TaskRepository, error) {
	input := entry.input
	if p.preserveCheckout {
		if err := p.finalizeCheckout(&input, entry.checkoutErr, snapshot); err != nil {
			return nil, err
		}
	}
	if input.CheckoutOptions != nil && entry.checkoutInputErr != nil {
		return nil, entry.checkoutInputErr
	}
	inherited := []TaskRepositoryInput{input}
	preserveTaskRepositoryPolicySnapshots(inherited, snapshot.Repositories)
	policy := entry.policy
	candidate := inherited[0].BranchPolicySnapshot
	if candidate != nil && candidate.ID == input.BranchPolicyID && candidate.RepositoryID == entry.repositoryID {
		policy = cloneBranchPolicy(candidate)
	} else if entry.policyErr != nil {
		return nil, entry.policyErr
	}
	return buildResolvedTaskRepositoryRow(entry.repositoryID, entry.baseBranch, index, input, policy)
}

func (p *preparedRepositoryReplacement) finalizeCheckout(input *TaskRepositoryInput, capabilityErr error, snapshot models.TaskRepositoryReplacementSnapshot) error {
	prior, err := matchingRepositoryCheckoutOptions(*input, snapshot.Repositories)
	if err != nil {
		return err
	}
	if input.CheckoutOptions == nil {
		input.CheckoutOptions = prior
		return nil
	}
	if reflect.DeepEqual(prior, input.CheckoutOptions) {
		return nil
	}
	if !p.checkoutMutabilityAvailable {
		return errors.New("checkout option mutability is unavailable")
	}
	if snapshot.EnvironmentExists {
		return errors.New("checkout options cannot change after environment creation")
	}
	return capabilityErr
}

func claimPreparedRepositorySlot(seen map[string]bool, row *models.TaskRepository, label string) error {
	key := row.RepositoryID + "\x00" + row.BaseBranch + "\x00" + row.CheckoutBranch
	if !seen[key] {
		seen[key] = true
		return nil
	}
	branch := row.CheckoutBranch
	if branch == "" {
		branch = row.BaseBranch
	}
	if branch == "" {
		return fmt.Errorf("repository %q is listed more than once for this task", label)
	}
	return fmt.Errorf("repository %q on branch %q is listed more than once for this task", label, branch)
}
