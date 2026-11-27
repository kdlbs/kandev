package gitlab

// PipelineJob represents a single CI job within a GitLab pipeline, as
// returned by GET /projects/:id/pipelines/:id/jobs. Used by the pass-rate
// summary (job counts) and by MR auto-fix (failing job names/URLs).
type PipelineJob struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Stage        string `json:"stage"`
	Status       string `json:"status"` // success, failed, running, pending, created, manual, skipped, canceled
	AllowFailure bool   `json:"allow_failure"`
	WebURL       string `json:"web_url"`
}

// Job status buckets used to roll a job list up into pass-rate counts and
// the auto-fix failing-job delta.
const (
	pipelineJobBucketPassed     = "passed"
	pipelineJobBucketInProgress = "in_progress"
	pipelineJobBucketFailed     = "failed"
)

// pipelineJobBucket classifies progress without changing raw execution status.
// The passed bucket includes skipped jobs, allowed failures and optional manual
// jobs. Other nonterminal jobs remain in progress even when allowed to fail.
func pipelineJobBucket(job PipelineJob) string {
	switch job.Status {
	case pipelineStatusSuccess, pipelineStatusSkipped:
		return pipelineJobBucketPassed
	case "manual":
		if job.AllowFailure {
			return pipelineJobBucketPassed
		}
		return pipelineJobBucketInProgress
	case pipelineStatusFailed, pipelineStatusCanceled:
		if job.AllowFailure {
			return pipelineJobBucketPassed
		}
		return pipelineJobBucketFailed
	default: // running, pending, created, scheduled, ...
		return pipelineJobBucketInProgress
	}
}

// summarizePipelineJobs reduces a job list to total/passing counts plus the
// jobs that count as failed (allow_failure jobs excluded).
func summarizePipelineJobs(jobs []PipelineJob) (total, passing int, failing []PipelineJob) {
	total = len(jobs)
	for _, job := range jobs {
		switch pipelineJobBucket(job) {
		case pipelineJobBucketPassed:
			passing++
		case pipelineJobBucketFailed:
			failing = append(failing, job)
		}
	}
	return total, passing, failing
}
