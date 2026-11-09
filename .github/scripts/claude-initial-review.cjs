'use strict';

/**
 * Read-only admission gate for the first automatic same-repository Claude
 * review.
 *
 * GitHub does not schedule pull_request workflows while a pull request has merge
 * conflicts, so the opening review can be lost when an author resolves those
 * conflicts later. This gate admits a synchronization event only while no
 * retained run has started the review action. Retained Actions history is the
 * attempt record; this module writes nothing.
 */

const WORKFLOW_FILE = 'claude-code-review.yml';
const REVIEW_JOB_NAME = 'claude-review-same-repo';
const REVIEW_STEP_NAME = 'Run Claude Code Review (App token, inline comments)';
const HISTORY_PAGE_SIZE = 100;
const MAX_HISTORY_PAGES = 20;
// The workflow-runs endpoint exposes at most 1,000 results per query. A larger
// declared total cannot prove which runs were omitted, so admission fails.
const MAX_WORKFLOW_RUN_RESULTS = 1000;
const MAX_WORKFLOW_RUN_PAGES = MAX_WORKFLOW_RUN_RESULTS / HISTORY_PAGE_SIZE;
const MAX_REPORTED_RUN_IDS = 5;

// Concurrency admits one run at a time per pull request. A newer push can
// already be pending behind the current run; it cannot start the action until
// this run releases the group and it repeats admission. Only an in-progress
// foreign run proves concurrent execution.
const PENDING_RUN_STATUSES = new Set([
  'action_required',
  'pending',
  'queued',
  'requested',
  'waiting',
]);

const REASON = Object.freeze({
  NO_PRIOR_ATTEMPT: 'no_prior_attempt',
  PRIOR_ATTEMPT_STARTED: 'prior_attempt_started',
  STALE_HEAD: 'stale_head',
  CLOSED_PR: 'closed_pr',
  HISTORY_UNAVAILABLE: 'history_unavailable',
  MALFORMED_HISTORY: 'malformed_history',
  HISTORY_INCOMPLETE: 'history_incomplete',
  AMBIGUOUS_ATTEMPT: 'ambiguous_attempt',
  INVALID_INPUT: 'invalid_input',
});

/** Raised when admission cannot safely decide to start a review. */
class AdmissionError extends Error {
  constructor(reason, detail) {
    const suffix = detail ? ` (${detail})` : '';
    super(`initial review admission failed: ${reason}${suffix}`);
    this.name = 'AdmissionError';
    this.reason = reason;
  }
}

/** Return true for plain objects that are not arrays. */
function isRecord(value) {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** Bound an error message for a workflow diagnostic. */
function boundedErrorText(error) {
  const message =
    error instanceof Error && typeof error.message === 'string'
      ? error.message
      : String(error);
  return message.length > 200 ? `${message.slice(0, 197)}...` : message;
}

/** Require a positive integer input. */
function requirePositiveInteger(value, label) {
  if (!Number.isInteger(value) || value <= 0) {
    throw new AdmissionError(REASON.INVALID_INPUT, label);
  }
}

/** Require a nonempty event head SHA. */
function requireHeadSha(value) {
  if (typeof value !== 'string' || value.length === 0) {
    throw new AdmissionError(REASON.INVALID_INPUT, 'event head');
  }
}

/**
 * Read bounded pages. A short page ends the read when the API does not report a
 * total. When it does, a page that disagrees with that total is incomplete
 * evidence, and a full final page at the bound fails instead of hiding another
 * page behind it.
 */
async function collectHistoryPages({
  label,
  fetchPage,
  selectItems,
  pageSize = HISTORY_PAGE_SIZE,
  maxPages = MAX_HISTORY_PAGES,
  maxResults = null,
}) {
  const items = [];
  for (let page = 1; page <= maxPages; page += 1) {
    let response;
    try {
      response = await fetchPage(page);
    } catch (error) {
      throw new AdmissionError(
        REASON.HISTORY_UNAVAILABLE,
        `${label} page ${page}: ${boundedErrorText(error)}`,
      );
    }

    const data = response && response.data;
    if (!isRecord(data)) {
      throw new AdmissionError(REASON.MALFORMED_HISTORY, `${label} page ${page}`);
    }

    const pageItems = selectItems(data);
    if (!Array.isArray(pageItems)) {
      throw new AdmissionError(REASON.MALFORMED_HISTORY, `${label} page ${page}`);
    }
    const total = data.total_count;
    if (
      total !== undefined &&
      (!Number.isInteger(total) || total < 0)
    ) {
      throw new AdmissionError(
        REASON.MALFORMED_HISTORY,
        `${label} total_count`,
      );
    }
    if (
      Number.isInteger(total) &&
      maxResults !== null &&
      total > maxResults
    ) {
      throw new AdmissionError(
        REASON.HISTORY_INCOMPLETE,
        `${label} exceeds the ${maxResults}-result API limit`,
      );
    }

    items.push(...pageItems);
    if (Number.isInteger(total)) {
      // The declared total is the completeness proof. A page that cannot
      // account for it, including an empty final page that drops a record, is
      // incomplete evidence rather than an empty history.
      if (items.length >= total) {
        return items;
      }
      if (pageItems.length < pageSize) {
        throw new AdmissionError(
          REASON.HISTORY_INCOMPLETE,
          `${label} page ${page} collected ${items.length} of ${total} records`,
        );
      }
      if (page === maxPages) {
        throw new AdmissionError(
          REASON.HISTORY_INCOMPLETE,
          `${label} exceeds ${maxPages} pages of ${pageSize}`,
        );
      }
      continue;
    }

    if (pageItems.length < pageSize) {
      return items;
    }
    if (page === maxPages) {
      throw new AdmissionError(
        REASON.HISTORY_INCOMPLETE,
        `${label} exceeds ${maxPages} pages of ${pageSize}`,
      );
    }
  }
  return items;
}

/** Validate the fields required to associate a workflow run with a PR. */
function validateRun(run) {
  if (!isRecord(run) || run.id === undefined || run.id === null) {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'workflow run');
  }
  if (typeof run.status !== 'string' || run.status.length === 0) {
    throw new AdmissionError(
      REASON.MALFORMED_HISTORY,
      `workflow run ${run.id} status`,
    );
  }
  if (!Array.isArray(run.pull_requests)) {
    throw new AdmissionError(
      REASON.MALFORMED_HISTORY,
      `workflow run ${run.id} associations`,
    );
  }
  for (const association of run.pull_requests) {
    if (!isRecord(association) || !Number.isInteger(association.number)) {
      throw new AdmissionError(
        REASON.MALFORMED_HISTORY,
        `workflow run ${run.id} association`,
      );
    }
  }
}

/** Return true when a run is associated with the pull request number. */
function isAssociatedWithPullRequest(run, pullNumber) {
  return run.pull_requests.some(
    (association) => association.number === pullNumber,
  );
}

/** Read associated workflow runs for a pull request, bounded by the API limit. */
async function listAssociatedRuns({ github, owner, repo, pullNumber, createdAt }) {
  const created = `>=${createdAt.slice(0, 10)}`;
  const runs = await collectHistoryPages({
    label: 'workflow runs',
    fetchPage: (page) =>
      github.rest.actions.listWorkflowRuns({
        owner,
        repo,
        workflow_id: WORKFLOW_FILE,
        event: 'pull_request',
        created,
        per_page: HISTORY_PAGE_SIZE,
        page,
        // Associations are the supported way to bind a run to a PR number.
        exclude_pull_requests: false,
      }),
    selectItems: (data) => data.workflow_runs,
    maxPages: MAX_WORKFLOW_RUN_PAGES,
    maxResults: MAX_WORKFLOW_RUN_RESULTS,
  });

  for (const run of runs) {
    validateRun(run);
  }
  return runs.filter((run) => isAssociatedWithPullRequest(run, pullNumber));
}

/**
 * A non-skipped step with a start timestamp consumes the attempt even when it
 * later fails or is cancelled. GitHub reports a skipped step as completed with
 * conclusion `skipped` and a nonempty start timestamp, so the conclusion must
 * be checked before the timestamp.
 */
function stepInvoked(step) {
  if (!isRecord(step) || typeof step.status !== 'string') {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'action step');
  }
  if (step.status === 'skipped' || step.conclusion === 'skipped') {
    return false;
  }
  if (typeof step.started_at === 'string' && step.started_at.length > 0) {
    return true;
  }
  if (step.status === 'completed') {
    throw new AdmissionError(
      REASON.MALFORMED_HISTORY,
      'completed action step without a start',
    );
  }
  return false;
}

/** Return true when a job's review step started and was not skipped. */
function jobInvokedAttempt(job) {
  if (!isRecord(job) || typeof job.name !== 'string') {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'workflow job');
  }
  if (job.name !== REVIEW_JOB_NAME) {
    return false;
  }
  if (typeof job.status !== 'string' || !Array.isArray(job.steps)) {
    throw new AdmissionError(
      REASON.MALFORMED_HISTORY,
      `${REVIEW_JOB_NAME} job`,
    );
  }

  const reviewStep = job.steps.find(
    (step) => isRecord(step) && step.name === REVIEW_STEP_NAME,
  );
  if (reviewStep !== undefined) {
    return stepInvoked(reviewStep);
  }
  if (job.status !== 'completed') {
    throw new AdmissionError(
      REASON.AMBIGUOUS_ATTEMPT,
      `${REVIEW_JOB_NAME} job has not reached a terminal state`,
    );
  }
  return false;
}

/** Read every job attempt for one workflow run. */
async function listRunJobs({ github, owner, repo, runId }) {
  return collectHistoryPages({
    label: `jobs for run ${runId}`,
    fetchPage: (page) =>
      github.rest.actions.listJobsForWorkflowRun({
        owner,
        repo,
        run_id: runId,
        // `all` retains every attempt in a rerun. A later skipped attempt must
        // not hide an earlier invocation.
        filter: 'all',
        per_page: HISTORY_PAGE_SIZE,
        page,
      }),
    selectItems: (data) => data.jobs,
  });
}

/**
 * A pending run can be a rerun of a run whose earlier attempt already invoked
 * the review. Inspect completed attempts only: the pending attempt has not
 * reached a terminal state and is not evidence by itself.
 */
async function pendingRerunInvoked({
  github,
  owner,
  repo,
  runId,
}) {
  const jobs = await listRunJobs({ github, owner, repo, runId });
  for (const job of jobs) {
    if (!isRecord(job) || typeof job.name !== 'string') {
      throw new AdmissionError(REASON.MALFORMED_HISTORY, 'workflow job');
    }
    if (job.status !== 'completed') {
      continue;
    }
    if (jobInvokedAttempt(job)) {
      return true;
    }
  }
  return false;
}

/** Find the first retained run that started the review action. */
async function findPriorAttempt({ github, owner, repo, runs, currentRunId }) {
  for (const run of runs) {
    if (String(run.id) === String(currentRunId)) {
      // An explicitly rerun workflow is a human action outside automatic
      // deduplication. Excluding the current run preserves that entry point.
      continue;
    }
    if (run.status === 'in_progress') {
      throw new AdmissionError(
        REASON.AMBIGUOUS_ATTEMPT,
        `run ${run.id} is still in progress`,
      );
    }
    if (PENDING_RUN_STATUSES.has(run.status)) {
      if (!Number.isInteger(run.run_attempt) || run.run_attempt < 1) {
        throw new AdmissionError(
          REASON.MALFORMED_HISTORY,
          `workflow run ${run.id} attempt`,
        );
      }
      if (
        run.run_attempt > 1 &&
        (await pendingRerunInvoked({ github, owner, repo, runId: run.id }))
      ) {
        return run.id;
      }
      // A genuinely new pending event waits behind the current run and repeats
      // admission when the concurrency group releases.
      continue;
    }
    if (run.status !== 'completed') {
      throw new AdmissionError(
        REASON.MALFORMED_HISTORY,
        `workflow run ${run.id} status`,
      );
    }

    const jobs = await listRunJobs({ github, owner, repo, runId: run.id });
    for (const job of jobs) {
      if (jobInvokedAttempt(job)) {
        return run.id;
      }
    }
  }
  return null;
}

/** Build a bounded admission decision. */
function decision(shouldReview, reason, runIds = []) {
  return {
    shouldReview,
    reason,
    runIds: runIds.slice(0, MAX_REPORTED_RUN_IDS),
  };
}

/** Read the current pull request with a bounded diagnostic on failure. */
async function readPullRequest({ github, owner, repo, pullNumber }) {
  let response;
  try {
    response = await github.rest.pulls.get({
      owner,
      repo,
      pull_number: pullNumber,
    });
  } catch (error) {
    throw new AdmissionError(
      REASON.HISTORY_UNAVAILABLE,
      `pull request read: ${boundedErrorText(error)}`,
    );
  }
  const pull = response && response.data;
  if (!isRecord(pull)) {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'pull request');
  }
  if (
    pull.number !== undefined &&
    Number(pull.number) !== pullNumber
  ) {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'pull request number');
  }
  return pull;
}

/**
 * Evaluate the event against the current pull request. A closed PR or a head
 * that no longer matches the event skips review; other states are malformed
 * evidence and fail closed.
 */
function evaluatePullRequest(pull, eventHeadSha) {
  if (pull.state === 'closed') {
    return decision(false, REASON.CLOSED_PR);
  }
  if (pull.state !== 'open') {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'pull request state');
  }
  if (!isRecord(pull.head) || typeof pull.head.sha !== 'string') {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'pull request head');
  }
  if (pull.head.sha !== eventHeadSha) {
    return decision(false, REASON.STALE_HEAD);
  }
  return null;
}

/**
 * Decide whether this pull-request event may start the automatic review.
 *
 * @param {object} options
 * @param {object} options.github Authenticated Octokit client.
 * @param {string} options.owner Repository owner.
 * @param {string} options.repo Repository name.
 * @param {number} options.pullNumber Event pull request number.
 * @param {string} options.eventHeadSha Event pull request head SHA.
 * @param {number|string} options.currentRunId Current workflow run ID.
 * @returns {Promise<{shouldReview: boolean, reason: string, runIds: (number|string)[]}>}
 */
async function shouldStartInitialReview({
  github,
  owner,
  repo,
  pullNumber,
  eventHeadSha,
  currentRunId,
}) {
  if (!isRecord(github) || !isRecord(github.rest) || !isRecord(github.rest.pulls)) {
    throw new AdmissionError(REASON.INVALID_INPUT, 'github client');
  }
  if (typeof owner !== 'string' || owner.length === 0) {
    throw new AdmissionError(REASON.INVALID_INPUT, 'owner');
  }
  if (typeof repo !== 'string' || repo.length === 0) {
    throw new AdmissionError(REASON.INVALID_INPUT, 'repo');
  }
  requirePositiveInteger(pullNumber, 'pull number');
  requireHeadSha(eventHeadSha);
  if (
    currentRunId === undefined ||
    currentRunId === null ||
    String(currentRunId).length === 0
  ) {
    throw new AdmissionError(REASON.INVALID_INPUT, 'current run');
  }

  const pull = await readPullRequest({ github, owner, repo, pullNumber });
  const initialDecision = evaluatePullRequest(pull, eventHeadSha);
  if (initialDecision !== null) {
    return initialDecision;
  }
  if (typeof pull.created_at !== 'string' || pull.created_at.length < 10) {
    throw new AdmissionError(REASON.MALFORMED_HISTORY, 'pull request creation time');
  }

  const runs = await listAssociatedRuns({
    github,
    owner,
    repo,
    pullNumber,
    createdAt: pull.created_at,
  });
  const priorRunId = await findPriorAttempt({
    github,
    owner,
    repo,
    runs,
    currentRunId,
  });
  if (priorRunId !== null) {
    return decision(false, REASON.PRIOR_ATTEMPT_STARTED, [priorRunId]);
  }

  // History reads can take many seconds. Re-read the PR at the final admission
  // boundary so an event that became stale or closed during those reads never
  // starts a review for an obsolete head.
  const currentPull = await readPullRequest({ github, owner, repo, pullNumber });
  const finalDecision = evaluatePullRequest(currentPull, eventHeadSha);
  if (finalDecision !== null) {
    return finalDecision;
  }
  return decision(true, REASON.NO_PRIOR_ATTEMPT);
}

module.exports = {
  AdmissionError,
  REASON,
  REVIEW_JOB_NAME,
  REVIEW_STEP_NAME,
  WORKFLOW_FILE,
  shouldStartInitialReview,
};
