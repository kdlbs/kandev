package github

import "context"

func revalidateCIRunPRAndSource(
	ctx context.Context,
	client ciRunActionsClient,
	binding *ciRunBinding,
	request *CIRunRequest,
	run *GitHubActionsRun,
	input RequestFreshCIRunInput,
) (*PR, error) {
	pr, err := client.GetPR(ctx, binding.Owner, binding.Repo, request.PRNumber)
	if err != nil {
		return nil, err
	}
	if pr != nil {
		request.ObservedPRHeadSHA = pr.HeadSHA
	}
	if err := verifyCIRunPR(pr, binding, input); err != nil {
		return nil, err
	}
	if !sourceRunMatches(run, pr, binding, input) {
		return nil, &CIRunRequestError{Class: CIRunFailureSourceRunMismatch}
	}
	return pr, nil
}
