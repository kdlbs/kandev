'use strict';

const assert = require('node:assert/strict');
const { test } = require('node:test');

const {
  AdmissionError,
  REASON,
  REVIEW_JOB_NAME,
  REVIEW_STEP_NAME,
  WORKFLOW_FILE,
  shouldStartInitialReview,
} = require('./claude-initial-review.cjs');

const PULL_NUMBER = 5208;
const RESOLUTION_HEAD = 'a'.repeat(40);
const OLD_HEAD = 'b'.repeat(40);
const LATER_HEAD = 'c'.repeat(40);
const CURRENT_RUN_ID = 9000;

const BASE_ARGS = {
  owner: 'kdlbs',
  repo: 'kandev',
  pullNumber: PULL_NUMBER,
  currentRunId: CURRENT_RUN_ID,
};

function pullRequest(overrides = {}) {
  return {
    number: PULL_NUMBER,
    state: 'open',
    created_at: '2026-11-08T22:04:41Z',
    head: { sha: RESOLUTION_HEAD },
    ...overrides,
  };
}

function associatedRun(id, overrides = {}) {
  return {
    id,
    status: 'completed',
    conclusion: 'success',
    event: 'pull_request',
    run_attempt: 1,
    pull_requests: [{ number: PULL_NUMBER }],
    ...overrides,
  };
}

function unrelatedRun(id, overrides = {}) {
  return associatedRun(id, {
    pull_requests: [{ number: PULL_NUMBER + 1 }],
    ...overrides,
  });
}

function reviewStep(overrides = {}) {
  return {
    name: REVIEW_STEP_NAME,
    status: 'completed',
    conclusion: 'success',
    started_at: '2026-11-08T22:05:00Z',
    completed_at: '2026-11-08T22:10:00Z',
    ...overrides,
  };
}

function reviewJob(steps, overrides = {}) {
  return {
    name: REVIEW_JOB_NAME,
    status: 'completed',
    conclusion: 'success',
    steps,
    ...overrides,
  };
}

function startedReviewJob() {
  return reviewJob([reviewStep()]);
}

function skippedActionJob() {
  // GitHub reports a skipped step as completed/skipped with a start timestamp.
  return reviewJob([
    reviewStep({ status: 'completed', conclusion: 'skipped' }),
  ]);
}

function checkoutOnlyJob() {
  return reviewJob([
    {
      name: 'Checkout PR head',
      status: 'completed',
      conclusion: 'success',
      started_at: '2026-11-08T22:04:50Z',
    },
  ]);
}

function skippedJob() {
  return reviewJob([], { status: 'completed', conclusion: 'skipped' });
}

function fillerJob(index) {
  return {
    name: `unrelated-job-${index}`,
    status: 'completed',
    conclusion: 'success',
    steps: [],
  };
}

function fillerRuns(count) {
  return Array.from({ length: count }, (_, index) =>
    unrelatedRun(10_000 + index),
  );
}

function deferred() {
  let resolve;
  const promise = new Promise((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function fakeGithub({ pull = pullRequest(), runs = [], jobs = {} } = {}) {
  const calls = { pullRequest: [], runs: [], jobs: [] };
  const readPull = typeof pull === 'function' ? pull : () => pull;

  return {
    calls,
    rest: {
      pulls: {
        get: async (args) => {
          calls.pullRequest.push(args);
          const value = readPull();
          if (value instanceof Error) throw value;
          return { data: value };
        },
      },
      actions: {
        listWorkflowRuns: async (args) => {
          calls.runs.push(args);
          if (runs instanceof Error) throw runs;
          const start = (args.page - 1) * args.per_page;
          return {
            data: {
              total_count: runs.length,
              workflow_runs: runs.slice(start, start + args.per_page),
            },
          };
        },
        listJobsForWorkflowRun: async (args) => {
          calls.jobs.push(args);
          const value = jobs[args.run_id];
          if (value instanceof Error) throw value;
          const all = value ?? [];
          const start = (args.page - 1) * args.per_page;
          return {
            data: {
              total_count: all.length,
              jobs: all.slice(start, start + args.per_page),
            },
          };
        },
      },
    },
  };
}

async function expectAdmissionError(promise, reason) {
  await assert.rejects(promise, (error) => {
    assert.ok(error instanceof AdmissionError, `unexpected error: ${error}`);
    assert.equal(error.reason, reason);
    return true;
  });
}

async function decide(github, overrides = {}) {
  return shouldStartInitialReview({
    ...BASE_ARGS,
    github,
    eventHeadSha: RESOLUTION_HEAD,
    ...overrides,
  });
}

// @covers AC-CI-CLAUDE-INITIAL-REVIEW-001.1
test('admits an opening review with no retained history', async () => {
  const github = fakeGithub();

  const result = await decide(github);

  assert.deepEqual(result, {
    shouldReview: true,
    reason: REASON.NO_PRIOR_ATTEMPT,
    runIds: [],
  });
  const [request] = github.calls.runs;
  assert.equal(request.workflow_id, WORKFLOW_FILE);
  assert.equal(request.event, 'pull_request');
  assert.equal(request.created, '>=2026-11-08');
  assert.equal(request.exclude_pull_requests, false);
  assert.equal(request.per_page, 100);
  assert.equal(request.page, 1);
  assert.equal(request.branch, undefined, 'history must not filter by branch');
});

// @covers AC-CI-CLAUDE-INITIAL-REVIEW-001.2
test('conflict resolution admits exactly one staged initial review', async () => {
  // The opening event is suppressed by GitHub while the PR conflicts, so it
  // leaves no run behind. The resolution push is the first scheduled event.
  const history = { runs: [], jobs: {} };
  let head = RESOLUTION_HEAD;
  let invocations = 0;
  const github = fakeGithub({
    pull: () => pullRequest({ head: { sha: head } }),
    runs: history.runs,
    jobs: history.jobs,
  });

  const resolution = await decide(github, { currentRunId: 1001 });
  assert.equal(resolution.shouldReview, true);
  if (resolution.shouldReview) {
    invocations += 1;
    history.runs.push(associatedRun(1001));
    history.jobs[1001] = [startedReviewJob()];
  }

  head = LATER_HEAD;
  const laterPush = await decide(github, {
    eventHeadSha: LATER_HEAD,
    currentRunId: 1002,
  });

  assert.deepEqual(laterPush, {
    shouldReview: false,
    reason: REASON.PRIOR_ATTEMPT_STARTED,
    runIds: [1001],
  });
  assert.equal(invocations, 1);
});

// @covers AC-CI-CLAUDE-INITIAL-REVIEW-001.3
test('suppresses a later push after an attempt on an older commit', async () => {
  const github = fakeGithub({
    runs: [
      associatedRun(77, {
        head_sha: OLD_HEAD,
        head_branch: 'feature/conflict-resolution',
      }),
    ],
    jobs: { 77: [startedReviewJob()] },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.deepEqual(result.runIds, [77]);
  assert.equal(github.calls.runs[0].head_sha, undefined);
  assert.equal(github.calls.runs[0].branch, undefined);
});

for (const conclusion of ['failure', 'cancelled']) {
  test(`suppresses after a ${conclusion} action attempt`, async () => {
    const github = fakeGithub({
      runs: [associatedRun(81)],
      jobs: {
        81: [reviewJob([reviewStep({ conclusion, completed_at: null })])],
      },
    });

    const result = await decide(github);

    assert.equal(result.shouldReview, false);
    assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
  });
}

test('admits when a retained job never started the review action', async () => {
  for (const job of [skippedJob(), checkoutOnlyJob(), skippedActionJob()]) {
    const github = fakeGithub({
      runs: [associatedRun(91)],
      jobs: { 91: [job] },
    });

    const result = await decide(github);
    assert.equal(result.shouldReview, true, JSON.stringify(job));
    assert.equal(result.reason, REASON.NO_PRIOR_ATTEMPT);
  }
});

test('a stale event that skipped the action does not consume the attempt', async () => {
  const history = { runs: [], jobs: {} };
  let head = RESOLUTION_HEAD;
  let invocations = 0;
  const github = fakeGithub({
    pull: () => pullRequest({ head: { sha: head } }),
    runs: history.runs,
    jobs: history.jobs,
  });

  // An earlier event became stale after its admission started, so its gated
  // action step is completed/skipped with a real start timestamp.
  history.runs.push(associatedRun(401, { head_sha: OLD_HEAD }));
  history.jobs[401] = [skippedActionJob()];

  const resolution = await decide(github, { currentRunId: 402 });
  assert.equal(resolution.shouldReview, true);
  if (resolution.shouldReview) {
    invocations += 1;
    history.runs.push(associatedRun(402));
    history.jobs[402] = [startedReviewJob()];
  }

  head = LATER_HEAD;
  const laterPush = await decide(github, {
    eventHeadSha: LATER_HEAD,
    currentRunId: 403,
  });

  assert.equal(laterPush.shouldReview, false);
  assert.equal(laterPush.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.equal(invocations, 1);
});

test('a pending rerun does not hide a consumed attempt', async () => {
  const github = fakeGithub({
    runs: [associatedRun(601, { status: 'pending', run_attempt: 2 })],
    jobs: {
      601: [
        { name: REVIEW_JOB_NAME, status: 'queued', steps: [] },
        startedReviewJob(),
      ],
    },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.deepEqual(result.runIds, [601]);
  assert.equal(github.calls.jobs.length, 1);
  assert.equal(github.calls.jobs[0].filter, 'all');
});

test('ignores a pending rerun whose earlier attempt did not invoke', async () => {
  const github = fakeGithub({
    runs: [associatedRun(602, { status: 'pending', run_attempt: 2 })],
    jobs: { 602: [skippedJob()] },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, true);
  assert.equal(result.reason, REASON.NO_PRIOR_ATTEMPT);
});

test('fails closed when a pending run has no attempt number', async () => {
  const github = fakeGithub({
    runs: [associatedRun(603, { status: 'pending', run_attempt: undefined })],
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('ignores retained runs for another pull request', async () => {
  const github = fakeGithub({
    runs: [unrelatedRun(101), associatedRun(102)],
    jobs: {
      101: [startedReviewJob()],
      102: [skippedJob()],
    },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, true);
  assert.equal(github.calls.jobs.length, 1);
  assert.equal(github.calls.jobs[0].run_id, 102);
});

test('excludes the current run so an explicit rerun is admitted', async () => {
  const github = fakeGithub({
    runs: [associatedRun(CURRENT_RUN_ID)],
    jobs: { [CURRENT_RUN_ID]: [startedReviewJob()] },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, true);
  assert.equal(result.reason, REASON.NO_PRIOR_ATTEMPT);
  assert.equal(github.calls.jobs.length, 0);
});

test('reads every page of run history', async () => {
  const github = fakeGithub({
    runs: [...fillerRuns(100), associatedRun(202)],
    jobs: { 202: [startedReviewJob()] },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.deepEqual(
    github.calls.runs.map((call) => call.page),
    [1, 2],
  );
});

test('reads every page of job history with filter all', async () => {
  const jobs = [...Array.from({ length: 100 }, (_, i) => fillerJob(i))];
  jobs.push(startedReviewJob());
  const github = fakeGithub({
    runs: [associatedRun(203)],
    jobs: { 203: jobs },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.deepEqual(
    github.calls.jobs.map((call) => call.page),
    [1, 2],
  );
  for (const call of github.calls.jobs) {
    assert.equal(call.filter, 'all');
  }
});

test('partitions oversized run history and detects a prior review in an older range', async () => {
  const github = fakeGithub({ jobs: { 204: [startedReviewJob()] } });
  github.rest.actions.listWorkflowRuns = async ({ created }) => {
    if (!created.includes('..')) {
      return { data: { total_count: 1001, workflow_runs: [] } };
    }
    const [start, end] = created.split('..');
    const prior = '2026-11-08T23:00:00Z';
    const runs = start <= prior && prior <= end ? [associatedRun(204)] : [];
    return { data: { total_count: runs.length, workflow_runs: runs } };
  };
  const result = await decide(github);
  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
});

test('fails closed when a partitioned history range cannot be read', async () => {
  const github = fakeGithub();
  github.rest.actions.listWorkflowRuns = async ({ created }) => {
    if (!created.includes('..')) {
      return { data: { total_count: 1001, workflow_runs: [] } };
    }
    throw new Error('API unavailable');
  };
  await expectAdmissionError(decide(github), REASON.HISTORY_UNAVAILABLE);
});

test('fails closed when run history exceeds the API result limit', async () => {
  const github = fakeGithub({ runs: fillerRuns(1001) });

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.ok(github.calls.runs.length > 1);
});

test('fails closed when the run page bound is reached without a total', async () => {
  const github = fakeGithub();
  let pages = 0;
  github.rest.actions.listWorkflowRuns = async () => {
    pages += 1;
    return { data: { workflow_runs: fillerRuns(100) } };
  };

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.equal(pages, 10);
});

test('fails closed when job history exceeds the page bound', async () => {
  const jobs = Array.from({ length: 2001 }, (_, i) => fillerJob(i));
  const github = fakeGithub({
    runs: [associatedRun(204)],
    jobs: { 204: jobs },
  });

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.equal(github.calls.jobs.length, 20);
});

test('accepts a full final page when total_count proves completeness', async () => {
  const github = fakeGithub({ runs: fillerRuns(1000) });

  const result = await decide(github);

  assert.equal(result.shouldReview, true);
  assert.equal(result.reason, REASON.NO_PRIOR_ATTEMPT);
  assert.equal(github.calls.runs.length, 10);
});

test('fails closed when a page disagrees with total_count', async () => {
  const github = fakeGithub();
  github.rest.actions.listWorkflowRuns = async () => ({
    data: { total_count: 150, workflow_runs: fillerRuns(50) },
  });

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
});

test('fails closed when the declared total exceeds the collected runs', async () => {
  const github = fakeGithub();
  github.rest.actions.listWorkflowRuns = async () => ({
    data: { total_count: 1, workflow_runs: [] },
  });

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
});

test('fails closed when an empty final run page drops a record', async () => {
  const github = fakeGithub();
  let pages = 0;
  github.rest.actions.listWorkflowRuns = async ({ page }) => {
    pages += 1;
    return {
      data: {
        total_count: 101,
        workflow_runs: page === 1 ? fillerRuns(100) : [],
      },
    };
  };

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.equal(pages, 2);
});

test('fails closed when a short final run page drops a record', async () => {
  const github = fakeGithub();
  let pages = 0;
  github.rest.actions.listWorkflowRuns = async ({ page }) => {
    pages += 1;
    return {
      data: {
        total_count: 102,
        workflow_runs: page === 1 ? fillerRuns(100) : [unrelatedRun(20_001)],
      },
    };
  };

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.equal(pages, 2);
});

test('fails closed when the declared total exceeds the collected jobs', async () => {
  const github = fakeGithub({ runs: [associatedRun(221)] });
  github.rest.actions.listJobsForWorkflowRun = async () => ({
    data: { total_count: 1, jobs: [] },
  });

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
});

test('fails closed when an empty final job page drops a record', async () => {
  const github = fakeGithub({ runs: [associatedRun(222)] });
  let pages = 0;
  github.rest.actions.listJobsForWorkflowRun = async ({ page }) => {
    pages += 1;
    return {
      data: {
        total_count: 101,
        jobs: page === 1
          ? Array.from({ length: 100 }, (_, index) => fillerJob(index))
          : [],
      },
    };
  };

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.equal(pages, 2);
});

test('fails closed when a short final job page drops a record', async () => {
  const github = fakeGithub({ runs: [associatedRun(223)] });
  let pages = 0;
  github.rest.actions.listJobsForWorkflowRun = async ({ page }) => {
    pages += 1;
    return {
      data: {
        total_count: 102,
        jobs: page === 1
          ? Array.from({ length: 100 }, (_, index) => fillerJob(index))
          : [fillerJob(200)],
      },
    };
  };

  await expectAdmissionError(decide(github), REASON.HISTORY_INCOMPLETE);
  assert.equal(pages, 2);
});

test('fails closed on a malformed total_count', async () => {
  const github = fakeGithub();
  github.rest.actions.listWorkflowRuns = async () => ({
    data: { total_count: '150', workflow_runs: [] },
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('fails closed on a negative total_count', async () => {
  const github = fakeGithub();
  github.rest.actions.listWorkflowRuns = async () => ({
    data: { total_count: -1, workflow_runs: [] },
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

// @covers AC-CI-CLAUDE-INITIAL-REVIEW-001.5
test('fails closed on malformed run history', async () => {
  const github = fakeGithub();
  github.rest.actions.listWorkflowRuns = async () => ({ data: {} });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('fails closed on malformed run associations', async () => {
  const github = fakeGithub({
    runs: [associatedRun(205, { pull_requests: undefined })],
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('fails closed on malformed job history', async () => {
  const github = fakeGithub({ runs: [associatedRun(206)] });
  github.rest.actions.listJobsForWorkflowRun = async () => ({
    data: { jobs: 'not-a-list' },
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('fails closed on a completed action step without a start', async () => {
  const github = fakeGithub({
    runs: [associatedRun(207)],
    jobs: {
      207: [reviewJob([reviewStep({ started_at: null })])],
    },
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('fails closed on GitHub API errors', async () => {
  const runError = fakeGithub({ runs: new Error('HTTP 502') });
  await expectAdmissionError(decide(runError), REASON.HISTORY_UNAVAILABLE);

  const pullError = fakeGithub({ pull: new Error('HTTP 500') });
  await expectAdmissionError(decide(pullError), REASON.HISTORY_UNAVAILABLE);

  const jobError = fakeGithub({
    runs: [associatedRun(208)],
    jobs: { 208: new Error('HTTP 503') },
  });
  await expectAdmissionError(decide(jobError), REASON.HISTORY_UNAVAILABLE);
});

// @covers AC-CI-CLAUDE-INITIAL-REVIEW-001.4
test('fails closed on a non-terminal retained run', async () => {
  const github = fakeGithub({
    runs: [associatedRun(209, { status: 'in_progress' })],
  });

  await expectAdmissionError(decide(github), REASON.AMBIGUOUS_ATTEMPT);
  assert.equal(github.calls.jobs.length, 0);
});

test('ignores a pending run behind the active concurrency group', async () => {
  const github = fakeGithub({
    runs: [associatedRun(211, { status: 'pending' })],
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, true);
  assert.equal(result.reason, REASON.NO_PRIOR_ATTEMPT);
  assert.equal(github.calls.jobs.length, 0);
});

test('fails closed on an unknown retained run status', async () => {
  const github = fakeGithub({
    runs: [associatedRun(212, { status: 'mystery' })],
  });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('ignores a non-terminal run for another pull request', async () => {
  const github = fakeGithub({
    runs: [unrelatedRun(210, { status: 'in_progress' })],
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, true);
});

test('skips a closed pull request without reading run history', async () => {
  const github = fakeGithub({ pull: pullRequest({ state: 'closed' }) });

  const result = await decide(github);

  assert.deepEqual(result, {
    shouldReview: false,
    reason: REASON.CLOSED_PR,
    runIds: [],
  });
  assert.equal(github.calls.runs.length, 0);
});

test('skips a stale event head without reading run history', async () => {
  const github = fakeGithub({
    pull: pullRequest({ head: { sha: LATER_HEAD } }),
  });

  const result = await decide(github);

  assert.deepEqual(result, {
    shouldReview: false,
    reason: REASON.STALE_HEAD,
    runIds: [],
  });
  assert.equal(github.calls.runs.length, 0);
});

test('rechecks the event head after history scanning', async () => {
  let head = RESOLUTION_HEAD;
  const started = deferred();
  const history = deferred();
  const github = fakeGithub({
    pull: () => pullRequest({ head: { sha: head } }),
  });
  github.rest.actions.listWorkflowRuns = async () => {
    head = LATER_HEAD;
    started.resolve();
    return history.promise;
  };

  const admission = decide(github);
  await started.promise;
  history.resolve({ data: { total_count: 0, workflow_runs: [] } });

  const result = await admission;

  assert.deepEqual(result, {
    shouldReview: false,
    reason: REASON.STALE_HEAD,
    runIds: [],
  });
});

test('rechecks the pull request state after history scanning', async () => {
  let state = 'open';
  const started = deferred();
  const history = deferred();
  const github = fakeGithub({ pull: () => pullRequest({ state }) });
  github.rest.actions.listWorkflowRuns = async () => {
    state = 'closed';
    started.resolve();
    return history.promise;
  };

  const admission = decide(github);
  await started.promise;
  history.resolve({ data: { total_count: 0, workflow_runs: [] } });

  const result = await admission;

  assert.deepEqual(result, {
    shouldReview: false,
    reason: REASON.CLOSED_PR,
    runIds: [],
  });
});

test('an obsolete event does not consume the new head review', async () => {
  const history = { runs: [], jobs: {} };
  let head = RESOLUTION_HEAD;
  let invocations = 0;
  const github = fakeGithub({
    pull: () => pullRequest({ head: { sha: head } }),
    runs: history.runs,
    jobs: history.jobs,
  });

  const started = deferred();
  const release = deferred();
  let pauseFirstScan = true;
  const listRuns = github.rest.actions.listWorkflowRuns;
  github.rest.actions.listWorkflowRuns = async (args) => {
    if (pauseFirstScan) {
      pauseFirstScan = false;
      started.resolve();
      await release.promise;
    }
    return listRuns(args);
  };

  const obsoleteEvent = decide(github, { currentRunId: 501 });
  await started.promise;
  head = LATER_HEAD;
  release.resolve();

  const obsolete = await obsoleteEvent;
  assert.equal(obsolete.reason, REASON.STALE_HEAD);

  const current = await decide(github, {
    eventHeadSha: LATER_HEAD,
    currentRunId: 502,
  });
  assert.equal(current.shouldReview, true);
  if (current.shouldReview) {
    invocations += 1;
    history.runs.push(associatedRun(502));
    history.jobs[502] = [startedReviewJob()];
  }

  head = 'd'.repeat(40);
  const later = await decide(github, {
    eventHeadSha: head,
    currentRunId: 503,
  });

  assert.equal(later.shouldReview, false);
  assert.equal(later.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.equal(invocations, 1);
});

test('suppresses a mixed history with one started attempt', async () => {
  const github = fakeGithub({
    runs: [
      associatedRun(301),
      associatedRun(302),
      associatedRun(303),
    ],
    jobs: {
      301: [skippedJob()],
      302: [startedReviewJob()],
      303: [checkoutOnlyJob()],
    },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
  assert.deepEqual(result.runIds, [302]);
});

test('a later skipped rerun attempt does not hide an earlier invocation', async () => {
  const github = fakeGithub({
    runs: [associatedRun(304)],
    jobs: {
      304: [startedReviewJob(), skippedJob()],
    },
  });

  const result = await decide(github);

  assert.equal(result.shouldReview, false);
  assert.equal(result.reason, REASON.PRIOR_ATTEMPT_STARTED);
});

test('fails closed on a mismatched pull request number', async () => {
  const github = fakeGithub({ pull: pullRequest({ number: PULL_NUMBER + 1 }) });

  await expectAdmissionError(decide(github), REASON.MALFORMED_HISTORY);
});

test('rejects incomplete admission inputs', async () => {
  const github = fakeGithub();

  await expectAdmissionError(
    shouldStartInitialReview({
      ...BASE_ARGS,
      github,
      eventHeadSha: RESOLUTION_HEAD,
      pullNumber: 0,
    }),
    REASON.INVALID_INPUT,
  );
  await expectAdmissionError(
    shouldStartInitialReview({
      ...BASE_ARGS,
      github,
      eventHeadSha: RESOLUTION_HEAD,
      currentRunId: '',
    }),
    REASON.INVALID_INPUT,
  );
});
