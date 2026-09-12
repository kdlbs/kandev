import { render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  resetContributionHistoryExplanationForTests,
  useContributionHistoryExplanation,
  type ContributionHistoryExplanationTarget,
} from "./use-contribution-history-explanation";

const requestMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (error: unknown) => void;
};

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

const target: ContributionHistoryExplanationTarget = {
  sessionId: "session-1",
  workspaceId: "workspace-1",
  repositoryScope: "frontend",
  branch: "feature/one",
  selectedPRKey: "acme/frontend/42",
  expectedLocalHead: "a".repeat(40),
  expectedRemoteHead: "b".repeat(40),
};

const explanation = {
  repo: "frontend",
  branch: "feature/one",
  expected_local_head: target.expectedLocalHead,
  expected_remote_head: target.expectedRemoteHead,
  kind: "local_rebase",
  reason: "matched_reflog",
  onto_head: "c".repeat(40),
  task_commit_count: 5,
  published_commit_count: 5,
  new_base_commit_count: 29,
} as const;

function SnapshotProbe({
  target: currentTarget,
  enabled,
  onRender,
}: {
  target: ContributionHistoryExplanationTarget;
  enabled: boolean;
  onRender: (snapshot: ReturnType<typeof useContributionHistoryExplanation>) => void;
}) {
  const snapshot = useContributionHistoryExplanation(currentTarget, enabled);
  onRender(snapshot);
  return null;
}

beforeEach(() => {
  requestMock.mockReset();
  resetContributionHistoryExplanationForTests();
});

afterEach(() => {
  resetContributionHistoryExplanationForTests();
});

describe("useContributionHistoryExplanation", () => {
  it("does not observe history until a confirmed explanation surface opens", () => {
    const hook = renderHook(({ enabled }) => useContributionHistoryExplanation(target, enabled), {
      initialProps: { enabled: false },
    });

    expect(requestMock).not.toHaveBeenCalled();

    hook.rerender({ enabled: true });

    expect(requestMock).toHaveBeenCalledOnce();
    expect(requestMock).toHaveBeenCalledWith(
      "worktree.contribution_history_explanation",
      {
        session_id: "session-1",
        repo: "frontend",
        branch: "feature/one",
        expected_local_head: target.expectedLocalHead,
        expected_remote_head: target.expectedRemoteHead,
      },
      expect.any(Number),
    );
  });

  it("shares one request and result across mounted consumers with the same identity", async () => {
    const pending = deferred<typeof explanation>();
    requestMock.mockReturnValue(pending.promise);
    const first = renderHook(() => useContributionHistoryExplanation(target, true));
    const second = renderHook(() => useContributionHistoryExplanation(target, true));

    expect(requestMock).toHaveBeenCalledOnce();
    pending.resolve(explanation);

    await waitFor(() => {
      expect(first.result.current.explanation).toEqual(explanation);
      expect(second.result.current.explanation).toEqual(explanation);
      expect(first.result.current.status).toBe("ready");
      expect(second.result.current.status).toBe("ready");
    });
  });

  it("releases the result when the last consumer dismisses the explanation", async () => {
    requestMock.mockResolvedValue(explanation);
    const hook = renderHook(({ enabled }) => useContributionHistoryExplanation(target, enabled), {
      initialProps: { enabled: true },
    });

    await waitFor(() => expect(hook.result.current.status).toBe("ready"));
    hook.rerender({ enabled: false });
    hook.rerender({ enabled: true });

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
  });

  it("discards a response after the requested head identity changes", async () => {
    const oldRequest = deferred<typeof explanation>();
    const newRequest = deferred<typeof explanation>();
    requestMock.mockReturnValueOnce(oldRequest.promise).mockReturnValueOnce(newRequest.promise);
    const hook = renderHook(
      ({ currentTarget }) => useContributionHistoryExplanation(currentTarget, true),
      { initialProps: { currentTarget: target } },
    );

    const nextTarget = { ...target, expectedLocalHead: "d".repeat(40) };
    hook.rerender({ currentTarget: nextTarget });
    oldRequest.resolve(explanation);

    await waitFor(() => expect(requestMock).toHaveBeenCalledTimes(2));
    expect(hook.result.current.explanation).toBeNull();

    newRequest.resolve({
      ...explanation,
      expected_local_head: nextTarget.expectedLocalHead,
    });
    await waitFor(() =>
      expect(hook.result.current.explanation?.expected_local_head).toBe(
        nextTarget.expectedLocalHead,
      ),
    );
  });

  it("returns a neutral snapshot during a provider refresh render", async () => {
    const pending = deferred<typeof explanation>();
    requestMock.mockReturnValueOnce(pending.promise).mockResolvedValueOnce({
      ...explanation,
      expected_remote_head: "e".repeat(40),
    });
    const renders: Array<ReturnType<typeof useContributionHistoryExplanation>> = [];
    const view = render(
      <SnapshotProbe target={target} enabled onRender={(snapshot) => renders.push(snapshot)} />,
    );
    pending.resolve(explanation);

    await waitFor(() => expect(renders.at(-1)?.status).toBe("ready"));
    const nextTarget = { ...target, expectedRemoteHead: "e".repeat(40) };
    const renderCountBeforeRefresh = renders.length;
    view.rerender(
      <SnapshotProbe target={nextTarget} enabled onRender={(snapshot) => renders.push(snapshot)} />,
    );

    expect(renders[renderCountBeforeRefresh]?.status).toBe("idle");
    expect(renders[renderCountBeforeRefresh]?.explanation).toBeNull();
    await waitFor(() => expect(renders.at(-1)?.status).toBe("ready"));
    expect(renders.at(-1)?.explanation?.expected_remote_head).toBe(nextTarget.expectedRemoteHead);
  });
});

describe("useContributionHistoryExplanation identity reuse", () => {
  it("does not restart an identical request for a new target object", () => {
    const pending = deferred<typeof explanation>();
    requestMock.mockReturnValue(pending.promise);
    const hook = renderHook(
      ({ currentTarget }) => useContributionHistoryExplanation(currentTarget, true),
      { initialProps: { currentTarget: target } },
    );

    hook.rerender({ currentTarget: { ...target } });

    expect(requestMock).toHaveBeenCalledOnce();
  });

  it("shares an observation when only the selected PR identity changes", () => {
    const pending = deferred<typeof explanation>();
    requestMock.mockReturnValue(pending.promise);
    const hook = renderHook(
      ({ currentTarget }) => useContributionHistoryExplanation(currentTarget, true),
      { initialProps: { currentTarget: target } },
    );

    hook.rerender({
      currentTarget: { ...target, selectedPRKey: "acme/frontend/43" },
    });

    expect(requestMock).toHaveBeenCalledOnce();
  });

  it("keeps separate repository and PR identities isolated", () => {
    requestMock.mockResolvedValue(explanation);
    const otherTarget = {
      ...target,
      repositoryScope: "backend",
      selectedPRKey: "acme/backend/42",
    };

    renderHook(() => useContributionHistoryExplanation(target, true));
    renderHook(() => useContributionHistoryExplanation(otherTarget, true));

    expect(requestMock).toHaveBeenCalledTimes(2);
    expect(requestMock.mock.calls.map(([action, payload]) => [action, payload])).toEqual([
      ["worktree.contribution_history_explanation", expect.objectContaining({ repo: "frontend" })],
      ["worktree.contribution_history_explanation", expect.objectContaining({ repo: "backend" })],
    ]);
  });

  it("does not publish a late result after all consumers cancel", async () => {
    const pending = deferred<typeof explanation>();
    requestMock.mockReturnValue(pending.promise);
    const hook = renderHook(() => useContributionHistoryExplanation(target, true));
    hook.unmount();
    pending.resolve(explanation);

    await Promise.resolve();
    expect(requestMock).toHaveBeenCalledOnce();
  });
});
