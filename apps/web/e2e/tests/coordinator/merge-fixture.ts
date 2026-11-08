// Shared by merge.spec.ts and mobile-merge.spec.ts. The merge reads (settings member,
// proposal detail, change state, dependencies) are stubbed at the network edge: their
// wire contract is proven against the real backend by the Go route tests, and what these
// specs prove is the browser rendering of it, including states (an inactive principal,
// an unconfirmed merge, a failed provider read) a mock provider cannot produce on demand.
import type { Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";

export const MERGE_ENV = {
  KANDEV_FEATURES_COORDINATOR: "true",
  KANDEV_FEATURES_COORDINATOR_PHASE2: "true",
  KANDEV_FEATURES_COORDINATOR_PHASE3: "true",
  KANDEV_FEATURES_COORDINATOR_MERGE: "true",
};

export async function enableMergeFeatures(backend: BackendContext): Promise<() => Promise<void>> {
  return backend.useEnv(MERGE_ENV);
}

export type StubRepo = {
  repository_id: string;
  name: string;
  policy: "denied" | "requires_approval" | "automatic";
  automatic_active: boolean;
};

export type StubObservation = {
  repository_id: string;
  branch: string;
  pipeline: "enforced" | "not_enforced" | "unknown";
  approvals: "enforced" | "not_enforced" | "unknown";
  threads: "enforced" | "not_enforced" | "unknown";
  observed_at: string;
};

export type SettingsStub = {
  repos: StubRepo[];
  protection: StubObservation[];
  puts: Array<Record<string, unknown>>;
  refreshes: number;
  /** When set, the next settings PUT answers 409 with this code. */
  putFailure: string | null;
  /** When set, settings reads answer 500 until cleared. */
  readFailing: boolean;
};

export function newSettingsStub(
  repos: StubRepo[],
  protection: StubObservation[] = [],
): SettingsStub {
  return { repos, protection, puts: [], refreshes: 0, putFailure: null, readFailing: false };
}

function mergeSettingsBody(base: Record<string, unknown>, stub: SettingsStub, revision: number) {
  return {
    ...base,
    policy_revision: revision,
    merge: { repositories: stub.repos, protection: stub.protection },
  };
}

/** Adds the merge member to the coordinator's settings read and records settings saves. */
export async function stubMergeSettings(
  page: Page,
  coordinatorId: string,
  stub: SettingsStub,
): Promise<void> {
  let revision = 1;
  let base: Record<string, unknown> = {};
  await page.route(new RegExp(`/coordinators/${coordinatorId}/settings$`), async (route) => {
    const method = route.request().method();
    if (method === "GET") {
      if (stub.readFailing) return route.fulfill({ status: 500, json: { error: "read_error" } });
      const response = await route.fetch();
      base = (await response.json()) as Record<string, unknown>;
      return route.fulfill({ response, json: mergeSettingsBody(base, stub, revision) });
    }
    if (method !== "PUT") return route.fallback();
    const body = route.request().postDataJSON() as {
      merge?: {
        repositories: Array<{
          repository_id: string;
          policy: StubRepo["policy"];
          replace_principal?: boolean;
        }>;
      };
    };
    if (!body.merge) return route.fallback();
    stub.puts.push(body as Record<string, unknown>);
    if (stub.putFailure) {
      const code = stub.putFailure;
      stub.putFailure = null;
      return route.fulfill({ status: 409, json: { code, error: code } });
    }
    for (const change of body.merge.repositories) {
      const repo = stub.repos.find((r) => r.repository_id === change.repository_id);
      if (!repo) continue;
      repo.policy = change.policy;
      if (change.replace_principal) repo.automatic_active = true;
    }
    revision += 1;
    return route.fulfill({ json: mergeSettingsBody(base, stub, revision) });
  });
  await page.route(
    new RegExp(`/coordinators/${coordinatorId}/merge/protection/refresh$`),
    (route) => {
      stub.refreshes += 1;
      return route.fulfill({ json: { protection: stub.protection } });
    },
  );
}

export function mergeProposalBody(
  coordinatorId: string,
  workspaceId: string,
  taskId: string,
  over: Record<string, unknown> = {},
): Record<string, unknown> {
  const now = new Date().toISOString();
  return {
    id: "merge-proposal-1",
    coordinator_id: coordinatorId,
    workspace_id: workspaceId,
    status: "pending",
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: now,
    updated_at: now,
    kind: "merge",
    target_task_id: taskId,
    spec: {
      task_id: taskId,
      repository_id: "repo-1",
      change_key: "github|github.com|repo-1|601",
      head_sha: "0123456789abcdef",
      method: "squash",
    },
    final_spec: null,
    ...over,
  };
}

export const GREEN_LIVE = {
  state: "open",
  head_sha: "0123456789abcdef",
  draft: "met",
  pipeline: "green",
  mergeable: "met",
  conflict: "clear",
  unresolved_threads: 0,
  approvals: "met",
};

export type ProposalStub = {
  proposal: Record<string, unknown>;
  detail: Record<string, unknown>;
  approves: number;
  checks: number;
  releases: number;
};

/** Serves one merge proposal on the list, detail, approve, check and release routes. */
export async function stubMergeProposal(
  page: Page,
  coordinatorId: string,
  stub: ProposalStub,
): Promise<void> {
  const id = stub.proposal.id as string;
  const withDetail = () => ({ ...stub.proposal, merge: stub.detail });
  await page.route(new RegExp(`/coordinators/${coordinatorId}/proposals(\\?.*)?$`), (route) =>
    route.request().method() === "GET"
      ? route.fulfill({ json: { proposals: [stub.proposal] } })
      : route.fallback(),
  );
  await page.route(new RegExp(`/coordinators/${coordinatorId}/proposals/${id}$`), (route) =>
    route.fulfill({ json: withDetail() }),
  );
  await page.route(
    new RegExp(`/coordinators/${coordinatorId}/proposals/${id}/approve$`),
    async (route) => {
      stub.approves += 1;
      stub.proposal = { ...stub.proposal, status: "approved" };
      await route.fulfill({ json: stub.proposal });
    },
  );
  await page.route(
    new RegExp(`/coordinators/${coordinatorId}/proposals/${id}/merge/check$`),
    (route) => {
      stub.checks += 1;
      return route.fulfill({ json: stub.proposal });
    },
  );
  await page.route(
    new RegExp(`/coordinators/${coordinatorId}/proposals/${id}/merge/release$`),
    (route) => {
      stub.releases += 1;
      stub.proposal = { ...stub.proposal, status: "rejected", reject_reason: "unconfirmed" };
      return route.fulfill({ json: stub.proposal });
    },
  );
}

export type ChangeStub = Record<string, unknown>;

export function changeBody(over: Record<string, unknown> = {}): ChangeStub {
  return {
    key: "github|github.com|repo-1|601",
    provider: "github",
    host: "github.com",
    repository_id: "repo-1",
    number: 601,
    target_branch: "main",
    head_sha: "0123456789abcdef",
    state: "open",
    pipeline: "none",
    mergeable: "met",
    conflict: "met",
    approvals: "met",
    draft: "met",
    unresolved_threads: 0,
    behind: 3,
    unavailable: false,
    observed_at: "2026-05-01T10:00:00Z",
    ...over,
  };
}

export type QueueStub = {
  changes: ChangeStub[];
  changeReads: number;
  retryReads: number;
  deps: { refs: Array<{ task: string } | { change: string }>; revision: number };
  depPuts: Array<Record<string, unknown>>;
  /** The next dependency save answers 409 revision_conflict with this stored set. */
  staleOnce: { refs: Array<{ task: string } | { change: string }>; revision: number } | null;
};

export async function stubQueueMerge(
  page: Page,
  coordinatorId: string,
  taskId: string,
  stub: QueueStub,
): Promise<void> {
  const base = `/coordinators/${coordinatorId}/tasks/${taskId}`;
  await page.route(new RegExp(`${base}/changes(\\?.*)?$`), (route) => {
    stub.changeReads += 1;
    if (route.request().url().includes("retry=1")) stub.retryReads += 1;
    return route.fulfill({ json: { changes: stub.changes } });
  });
  await page.route(new RegExp(`${base}/dependencies$`), (route) => {
    if (route.request().method() === "GET") {
      return route.fulfill({ json: { refs: stub.deps.refs, revision: stub.deps.revision } });
    }
    stub.depPuts.push(route.request().postDataJSON() as Record<string, unknown>);
    if (stub.staleOnce) {
      stub.deps = stub.staleOnce;
      stub.staleOnce = null;
      return route.fulfill({
        status: 409,
        json: { code: "revision_conflict", error: "revision_conflict" },
      });
    }
    const put = route.request().postDataJSON() as { refs: QueueStub["deps"]["refs"] };
    stub.deps = { refs: put.refs, revision: stub.deps.revision + 1 };
    return route.fulfill({ json: stub.deps });
  });
}
