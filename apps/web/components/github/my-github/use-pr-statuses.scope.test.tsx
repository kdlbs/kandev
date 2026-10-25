import { useLayoutEffect, useRef, type ReactNode } from "react";
import { act, cleanup, render, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconChecklist } from "@tabler/icons-react";
import { StateProvider } from "@/components/state-provider";
import type { PRStatusRef } from "@/lib/api/domains/github-api";
import type { GitHubPR, GitHubPRStatus } from "@/lib/types/github";
import { PRList } from "./pr-list";
import { usePRStatuses } from "./use-pr-statuses";
import type { TaskPreset } from "./quick-task-launcher";

const transport = vi.hoisted(() => ({ batch: vi.fn() }));
vi.mock("@/lib/api/domains/github-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/github-api")>()),
  getPRStatusesBatch: (workspace: string, refs: PRStatusRef[]) => transport.batch(workspace, refs),
}));

const WORKSPACE_A = "workspace-a";
const WORKSPACE_B = "workspace-b";
const PR_KEY = "batch-owner/status-repo#7";
type Response = { statuses?: Record<string, GitHubPRStatus> };
type Request = {
  resolve: (response: Response) => void;
  reject: (error: Error) => void;
  settled: boolean;
};
const requests: Request[] = [];

function makePR(number = 7, title = "Current pull request"): GitHubPR {
  return {
    number,
    title,
    url: `https://github.com/batch-owner/status-repo/pull/${number}`,
    html_url: `https://github.com/batch-owner/status-repo/pull/${number}`,
    state: "open",
    head_branch: "feature",
    base_branch: "main",
    author_login: "octocat",
    repo_owner: "batch-owner",
    repo_name: "status-repo",
    draft: false,
    mergeable: true,
    additions: 1,
    deletions: 0,
    requested_reviewers: [],
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    merged_at: null,
    closed_at: null,
  };
}

function makeStatus(pr: GitHubPR, total = 8): GitHubPRStatus {
  return {
    pr,
    review_state: "approved",
    checks_state: "success",
    mergeable_state: "clean",
    review_count: 1,
    pending_review_count: 0,
    checks_total: total,
    checks_passing: total,
  };
}

function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </StateProvider>
  );
}

function mountStatuses(workspaceId: string | null, items: GitHubPR[], reactStrictMode = false) {
  const renders: Map<string, GitHubPRStatus>[] = [];
  const view = renderHook(
    (props: { workspaceId: string | null; items: GitHubPR[] }) => {
      const statuses = usePRStatuses(props.workspaceId, props.items);
      renders.push(statuses);
      return statuses;
    },
    { initialProps: { workspaceId, items }, wrapper: Providers, reactStrictMode },
  );
  return { ...view, renders };
}

async function acknowledge(index: number, status?: GitHubPRStatus) {
  await act(async () => {
    requests[index].settled = true;
    requests[index].resolve({ statuses: status ? { [PR_KEY]: status } : {} });
  });
}

async function fail(index: number) {
  await act(async () => {
    requests[index].settled = true;
    requests[index].reject(new Error("Controlled batch failure"));
  });
}

beforeEach(() => {
  transport.batch.mockReset();
  transport.batch.mockImplementation(
    () =>
      new Promise<Response>((resolve, reject) =>
        requests.push({ resolve, reject, settled: false }),
      ),
  );
});

// @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.4
describe("usePRStatuses requested batch: current failure and empty results", () => {
  it("keeps a failed current read empty without retrying unchanged inputs", async () => {
    const pr = makePR();
    const view = mountStatuses(WORKSPACE_A, [pr]);
    await fail(0);
    const empty = view.result.current;
    view.rerender({ workspaceId: WORKSPACE_A, items: [{ ...pr }] });
    expect(view.result.current.size).toBe(0);
    expect(view.result.current).toBe(empty);
    expect(transport.batch).toHaveBeenCalledTimes(1);
  });

  it.each([{ statuses: {} }, {}])("accepts a current empty response: %j", async (response) => {
    const pr = makePR();
    const view = mountStatuses(WORKSPACE_A, [pr]);
    await acknowledge(0, makeStatus(pr));
    view.rerender({ workspaceId: WORKSPACE_B, items: [pr] });
    await act(async () => {
      requests[1].settled = true;
      requests[1].resolve(response);
    });
    expect(view.result.current.size).toBe(0);
    const currentEmpty = view.result.current;
    view.rerender({ workspaceId: WORKSPACE_B, items: [{ ...pr }] });
    expect(view.result.current).toBe(currentEmpty);
    expect(transport.batch).toHaveBeenCalledTimes(2);
  });

  it("preserves empty failed-B return-A behavior without a recovery request", async () => {
    const pr = makePR();
    const view = mountStatuses(WORKSPACE_A, [pr]);
    await acknowledge(0, makeStatus(pr));
    view.rerender({ workspaceId: WORKSPACE_B, items: [pr] });
    await fail(1);
    expect(view.result.current.size).toBe(0);
    view.rerender({ workspaceId: WORKSPACE_A, items: [pr] });
    expect(view.result.current.size).toBe(0);
    expect(transport.batch).toHaveBeenCalledTimes(2);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.5
describe("usePRStatuses requested batch: cancelled reads", () => {
  it.each(["success", "failure"])(
    "ignores an obsolete A %s after current B acknowledges",
    async (outcome) => {
      const pr = makePR();
      const view = mountStatuses(WORKSPACE_A, [pr]);
      view.rerender({ workspaceId: WORKSPACE_B, items: [pr] });
      const current = makeStatus(pr, 5);
      await acknowledge(1, current);
      const map = view.result.current;
      if (outcome === "success") await acknowledge(0, makeStatus(pr));
      else await fail(0);
      expect(view.result.current).toBe(map);
      expect(view.result.current.get(PR_KEY)).toBe(current);
    },
  );

  it.each(["success", "failure"])("ignores pending A1 %s after A B A2", async (outcome) => {
    const pr = makePR();
    const view = mountStatuses(WORKSPACE_A, [pr]);
    view.rerender({ workspaceId: WORKSPACE_B, items: [pr] });
    view.rerender({ workspaceId: WORKSPACE_A, items: [pr] });
    expect(transport.batch).toHaveBeenCalledTimes(3);
    const current = makeStatus(pr, 5);
    await acknowledge(2, current);
    if (outcome === "success") await acknowledge(0, makeStatus(pr));
    else await fail(0);
    await acknowledge(1, makeStatus(pr, 3));
    expect(view.result.current.get(PR_KEY)).toBe(current);
  });

  it.each(["success", "failure"])(
    "settles an unmounted read's %s without changing a replacement",
    async (outcome) => {
      const pr = makePR();
      const previous = mountStatuses(WORKSPACE_A, [pr]);
      previous.unmount();
      const rendersAtUnmount = previous.renders.length;
      const replacement = mountStatuses(WORKSPACE_A, [pr]);
      const status = makeStatus(pr, 5);
      await acknowledge(1, status);
      if (outcome === "success") await acknowledge(0, makeStatus(pr));
      else await fail(0);
      expect(previous.renders).toHaveLength(rendersAtUnmount);
      expect(replacement.result.current.get(PR_KEY)).toBe(status);
      expect(transport.batch).toHaveBeenCalledTimes(2);
    },
  );
});

// @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.5
describe("usePRStatuses requested batch: lifecycle and isolation", () => {
  it.each(["success", "failure"])(
    "StrictMode ignores the cancelled first effect's %s",
    async (outcome) => {
      const pr = makePR();
      const view = mountStatuses(WORKSPACE_A, [pr], true);
      expect(transport.batch).toHaveBeenCalledTimes(2);
      if (outcome === "success") await acknowledge(0, makeStatus(pr));
      else await fail(0);
      expect(view.result.current.size).toBe(0);
      const current = makeStatus(pr, 5);
      await acknowledge(1, current);
      expect(view.result.current.get(PR_KEY)).toBe(current);
    },
  );

  // @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.6
  it("isolates same-key summaries across independent instances", async () => {
    const pr = makePR();
    const first = mountStatuses(WORKSPACE_A, [pr]);
    const second = mountStatuses(WORKSPACE_B, [pr]);
    const a = makeStatus(pr, 8);
    const b = makeStatus(pr, 5);
    await acknowledge(1, b);
    await acknowledge(0, a);
    expect(first.result.current.get(PR_KEY)).toBe(a);
    expect(second.result.current.get(PR_KEY)).toBe(b);
    first.rerender({ workspaceId: null, items: [pr] });
    expect(first.result.current.size).toBe(0);
    expect(second.result.current.get(PR_KEY)).toBe(b);
    expect(transport.batch).toHaveBeenCalledTimes(2);
  });
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const request of requests) {
      if (!request.settled) request.resolve({ statuses: {} });
    }
  });
  requests.length = 0;
});

// @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.1
describe("usePRStatuses requested batch: immediate reads", () => {
  it("hides previous workspace while current request is pending", async () => {
    const pr = makePR();
    const status = makeStatus(pr);
    const view = mountStatuses(WORKSPACE_A, [pr]);
    await acknowledge(0, status);
    expect(view.result.current.get(PR_KEY)).toBe(status);
    view.renders.length = 0;
    view.rerender({ workspaceId: WORKSPACE_B, items: [pr] });
    expect(view.renders[0].size).toBe(0);
    expect(view.result.current.size).toBe(0);
    expect(transport.batch).toHaveBeenLastCalledWith(WORKSPACE_B, [
      { owner: "batch-owner", repo: "status-repo", number: 7 },
    ]);
  });

  it.each(["add", "remove", "reorder"] as const)(
    "hides previous batch while new membership is pending: %s",
    async (change) => {
      const first = makePR();
      const second = makePR(8);
      const lists = {
        add: { initial: [first], next: [first, second] },
        remove: { initial: [first, second], next: [first] },
        reorder: { initial: [first, second], next: [second, first] },
      };
      const { initial, next } = lists[change];
      const view = mountStatuses(WORKSPACE_A, initial);
      await acknowledge(0, makeStatus(first));
      view.renders.length = 0;
      view.rerender({ workspaceId: WORKSPACE_A, items: next });
      expect(view.renders[0].size).toBe(0);
      expect(transport.batch).toHaveBeenCalledTimes(2);
    },
  );

  // @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.2
  it.each(["null workspace", "empty list"])(
    "clears the first render for %s without a request",
    async (change) => {
      const pr = makePR();
      const view = mountStatuses(WORKSPACE_A, [pr]);
      await acknowledge(0, makeStatus(pr));
      view.renders.length = 0;
      view.rerender({
        workspaceId: change === "null workspace" ? null : WORKSPACE_A,
        items: change === "empty list" ? [] : [pr],
      });
      expect(view.renders[0].size).toBe(0);
      expect(view.result.current.size).toBe(0);
      expect(transport.batch).toHaveBeenCalledTimes(1);
    },
  );
});

const preset: TaskPreset = {
  id: "review",
  label: "Review",
  hint: "Inspect pull request",
  icon: IconChecklist,
  prompt: ({ url }) => url,
};
type RowCommit = { title: string | null | undefined; oldBadge: boolean; taskAction: boolean };

function ListCommitProbe({
  workspaceId,
  items,
  commits,
}: {
  workspaceId: string | null;
  items: GitHubPR[];
  commits: RowCommit[];
}) {
  const container = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    const row = container.current?.querySelector('[data-testid="pr-row"]');
    commits.push({
      title: row?.querySelector("a")?.getAttribute("title"),
      oldBadge: Boolean(row?.querySelector('span[title="8/8"]')),
      taskAction: Boolean(row?.querySelector('[data-testid="pr-start-task-trigger"]')),
    });
  }, [workspaceId, items, commits]);
  return (
    <div ref={container}>
      <PRList
        workspaceId={workspaceId}
        items={items}
        loading={false}
        error={null}
        presets={[preset]}
        onStartTask={() => undefined}
      />
    </div>
  );
}

// @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.7
describe("usePRStatuses requested batch: actual PRList", () => {
  it("first scoped PRList commit omits prior green badge", async () => {
    const pr = makePR();
    const commits: RowCommit[] = [];
    const view = render(
      <ListCommitProbe workspaceId={WORKSPACE_A} items={[pr]} commits={commits} />,
      { wrapper: Providers },
    );
    await acknowledge(0, makeStatus(pr));
    expect(view.getByTitle("8/8")).toBeTruthy();
    commits.length = 0;
    const currentPR = makePR(7, "Workspace B pull request");
    view.rerender(
      <ListCommitProbe workspaceId={WORKSPACE_B} items={[currentPR]} commits={commits} />,
    );
    expect(commits[0]).toEqual({ title: currentPR.title, oldBadge: false, taskAction: true });
    expect(view.queryByTitle("8/8")).toBeNull();
    await acknowledge(1, makeStatus(currentPR, 5));
    expect(view.getByTitle("5/5")).toBeTruthy();
    expect(view.getByTitle(currentPR.title)).toBeTruthy();
  });

  it("renders current acknowledged batch", async () => {
    const pr = makePR();
    const view = render(<ListCommitProbe workspaceId={WORKSPACE_A} items={[pr]} commits={[]} />, {
      wrapper: Providers,
    });
    expect(view.queryByTitle("8/8")).toBeNull();
    expect(view.getByTestId("pr-start-task-trigger")).toBeTruthy();
    await acknowledge(0, makeStatus(pr));
    expect(view.getByTitle("8/8")).toBeTruthy();
    expect(view.getByTitle(pr.title)).toBeTruthy();
  });

  it("omits old badges in the first null-workspace row commit", async () => {
    const pr = makePR();
    const commits: RowCommit[] = [];
    const view = render(
      <ListCommitProbe workspaceId={WORKSPACE_A} items={[pr]} commits={commits} />,
      { wrapper: Providers },
    );
    await acknowledge(0, makeStatus(pr));
    expect(view.getByTitle("8/8")).toBeTruthy();
    commits.length = 0;
    view.rerender(<ListCommitProbe workspaceId={null} items={[pr]} commits={commits} />);
    expect(commits[0]).toEqual({ title: pr.title, oldBadge: false, taskAction: true });
    expect(transport.batch).toHaveBeenCalledTimes(1);
  });
});

// @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.3
describe("usePRStatuses requested batch: reuse", () => {
  it("retains Map and status identity for equal-content completed inputs", async () => {
    const pr = makePR();
    const status = makeStatus(pr);
    const view = mountStatuses(WORKSPACE_A, [pr]);
    await acknowledge(0, status);
    const completed = view.result.current;
    view.rerender({
      workspaceId: WORKSPACE_A,
      items: [{ ...pr, title: "New query title", updated_at: "2026-10-06T00:00:00Z" }],
    });
    expect(view.result.current).toBe(completed);
    expect(view.result.current.get(PR_KEY)).toBe(status);
    expect(transport.batch).toHaveBeenCalledTimes(1);
  });

  it("does not cancel a pending equal-content read", async () => {
    const pr = makePR();
    const status = makeStatus(pr);
    const view = mountStatuses(WORKSPACE_A, [pr]);
    view.rerender({ workspaceId: WORKSPACE_A, items: [{ ...pr }] });
    expect(transport.batch).toHaveBeenCalledTimes(1);
    await acknowledge(0, status);
    expect(view.result.current.get(PR_KEY)).toBe(status);
  });

  // @covers AC-INTEGRATIONS-GITHUB-BROWSE-STATUS-001.6
  it.each(["success", "failure"])(
    "reuses retained completed A after pending B and ignores B %s",
    async (outcome) => {
      const pr = makePR();
      const status = makeStatus(pr);
      const view = mountStatuses(WORKSPACE_A, [pr]);
      await acknowledge(0, status);
      const completed = view.result.current;
      view.rerender({ workspaceId: WORKSPACE_B, items: [pr] });
      expect(view.result.current.size).toBe(0);
      view.rerender({ workspaceId: WORKSPACE_A, items: [pr] });
      expect(view.result.current).toBe(completed);
      expect(transport.batch).toHaveBeenCalledTimes(2);
      if (outcome === "success") await acknowledge(1, makeStatus(pr, 5));
      else await fail(1);
      expect(view.result.current).toBe(completed);
    },
  );
});
