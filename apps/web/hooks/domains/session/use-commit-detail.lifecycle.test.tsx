import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { StrictMode, useLayoutEffect, type ReactNode } from "react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import {
  CommitDetailProtocolError,
  requestCommitDetail,
  type CommitDetailRequestResult,
} from "@/components/task/commit-detail-request";
import { scheduleFrontendErrorReport } from "@/lib/api/domains/frontend-error-log-api";
import { sessionId, taskId } from "@/lib/types/ids";
import { defaultState } from "@/lib/state/default-state";
import type { AppState, HydrationState } from "@/lib/state/store";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import { useCommitDetail, type UseCommitDetailResult } from "./use-commit-detail";

vi.mock("@/components/task/commit-detail-request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/components/task/commit-detail-request")>()),
  requestCommitDetail: vi.fn(),
}));
vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: vi.fn(),
}));

const toastTestId = "toast-message";
const target: CommitDetailTarget = {
  source: "github",
  workspaceId: "workspace",
  owner: "owner",
  repo: "repo",
  sha: "abcdef0123",
};

function deferred() {
  let resolve!: (result: CommitDetailRequestResult) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<CommitDetailRequestResult>((success, failure) => {
    resolve = success;
    reject = failure;
  });
  return { promise, resolve, reject };
}

function Reader({
  selected = target,
  onDetail,
  id = "reader",
}: {
  selected?: CommitDetailTarget;
  onDetail?: (detail: UseCommitDetailResult) => void;
  id?: string;
}) {
  const detail = useCommitDetail(selected);
  useLayoutEffect(() => onDetail?.(detail), [detail, onDetail]);
  return <div data-testid={id}>{JSON.stringify(detail)}</div>;
}

function App({ open }: { open: boolean }) {
  return <Providers>{open && <Reader />}</Providers>;
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.mocked(requestCommitDetail).mockReset();
  vi.mocked(scheduleFrontendErrorReport).mockClear();
});

// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.9
it("does not toast or report a failure after the reader closes", async () => {
  const request = deferred();
  vi.mocked(requestCommitDetail).mockReturnValue(request.promise);
  const view = render(<App open />);
  expect(requestCommitDetail).toHaveBeenCalledTimes(1);
  view.rerender(<App open={false} />);
  expect(screen.queryByTestId("reader")).toBeNull();
  await act(async () => request.reject(new Error("retired detail failure")));
  expect(screen.queryByTestId(toastTestId)).toBeNull();
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
});

// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.8
it("keeps current failure state, actual toast and frontend report", async () => {
  const request = deferred();
  vi.mocked(requestCommitDetail).mockReturnValue(request.promise);
  render(<App open />);
  await act(async () => request.reject(new Error("current detail failure")));
  expect(screen.getByTestId("reader").textContent).toContain('"error":"current detail failure"');
  expect(screen.getByTestId("reader").textContent).toContain('"loading":false');
  expect(screen.getByTestId(toastTestId).textContent).toContain("current detail failure");
  expect(scheduleFrontendErrorReport).toHaveBeenCalledWith(
    expect.objectContaining({ description: "current detail failure" }),
  );
});

function snapshot(id = "reader") {
  return JSON.parse(screen.getByTestId(id).textContent ?? "") as Omit<
    UseCommitDetailResult,
    "refetch"
  >;
}

function success(message = "current"): CommitDetailRequestResult {
  return {
    source: "github",
    success: true,
    files: {
      "file.ts": { path: "file.ts", status: "modified", staged: false, diff: message },
    },
    commit: {
      sha: target.sha,
      message,
      author_login: "owner",
      author_name: "Owner",
      author_date: "2026-10-03T12:00:00Z",
      additions: 1,
      deletions: 0,
      files_changed: 1,
      files: [],
    },
  };
}

function CaptureStore({ capture }: { capture: (store: StoreApi<AppState>) => void }) {
  const store = useAppStoreApi();
  useLayoutEffect(() => capture(store), [capture, store]);
  return null;
}

function Providers({
  children,
  initialState,
  captureStore,
}: {
  children: ReactNode;
  initialState?: HydrationState;
  captureStore?: (store: StoreApi<AppState>) => void;
}) {
  return (
    <StateProvider initialState={initialState}>
      <ToastProvider>
        {captureStore && <CaptureStore capture={captureStore} />}
        {children}
      </ToastProvider>
    </StateProvider>
  );
}

function LayoutAttempt({ retry }: { retry?: () => Promise<void> }) {
  useLayoutEffect(() => {
    void retry?.();
  }, [retry]);
  return null;
}

// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.9
it.each(["success", "failure"])("isolates a reopened reader from retired %s", async (outcome) => {
  const old = deferred();
  const current = deferred();
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(old.promise)
    .mockReturnValueOnce(current.promise);
  const view = render(<App open />);
  view.rerender(<App open={false} />);
  view.rerender(<App open />);
  await act(async () => {
    if (outcome === "success") old.resolve(success("retired"));
    else old.reject(new Error("retired"));
  });
  expect(snapshot()).toMatchObject({ files: null, commit: null, loading: true, error: null });
  expect(screen.queryByTestId(toastTestId)).toBeNull();
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
  await act(async () => current.resolve(success("reopened")));
  expect(snapshot()).toMatchObject({
    loading: false,
    error: null,
    commit: { message: "reopened" },
  });
});

it("denies a retained callback at the closing layout commit", async () => {
  const pending = deferred();
  let retry: (() => Promise<void>) | undefined;
  vi.mocked(requestCommitDetail).mockReturnValue(pending.promise);
  const capture = (detail: UseCommitDetailResult) => {
    retry = detail.refetch;
  };
  const view = render(
    <Providers>
      <Reader onDetail={capture} />
      <LayoutAttempt />
    </Providers>,
  );
  const retained = retry;
  view.rerender(
    <Providers>
      <LayoutAttempt retry={retained} />
    </Providers>,
  );
  expect(requestCommitDetail).toHaveBeenCalledTimes(1);
  await act(async () => {
    await retained?.();
    pending.reject(new Error("closed"));
  });
  expect(requestCommitDetail).toHaveBeenCalledTimes(1);
  expect(screen.queryByTestId(toastTestId)).toBeNull();
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
});

it.each(["success", "failure"])(
  "retires target A at layout commit before %s settles",
  async (outcome) => {
    const old = deferred();
    const next = deferred();
    let retry: (() => Promise<void>) | undefined;
    vi.mocked(requestCommitDetail)
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(next.promise);
    const capture = (detail: UseCommitDetailResult) => {
      retry = detail.refetch;
    };
    const view = render(
      <Providers>
        <Reader onDetail={capture} />
        <LayoutAttempt />
      </Providers>,
    );
    const retained = retry;
    view.rerender(
      <Providers>
        <Reader selected={{ ...target, sha: "other" }} onDetail={capture} />
        <LayoutAttempt retry={retained} />
      </Providers>,
    );
    expect(requestCommitDetail).toHaveBeenCalledTimes(2);
    await act(async () => {
      if (outcome === "success") old.resolve(success("retired target"));
      else old.reject(new Error("retired target"));
    });
    expect(snapshot()).toMatchObject({ loading: true, files: null, commit: null, error: null });
    expect(screen.queryByTestId(toastTestId)).toBeNull();
    expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
    await act(async () => next.resolve(success("next target")));
    expect(snapshot()).toMatchObject({ loading: false, commit: { message: "next target" } });
  },
);

it("does not revive a retained A callback when switching A to B to A", async () => {
  const first = deferred();
  const second = deferred();
  const third = deferred();
  let retry: (() => Promise<void>) | undefined;
  const capture = (detail: UseCommitDetailResult) => {
    retry = detail.refetch;
  };
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(second.promise)
    .mockReturnValueOnce(third.promise);
  const view = render(
    <Providers>
      <Reader onDetail={capture} />
    </Providers>,
  );
  const retained = retry;
  view.rerender(
    <Providers>
      <Reader selected={{ ...target, sha: "B" }} onDetail={capture} />
    </Providers>,
  );
  view.rerender(
    <Providers>
      <Reader onDetail={capture} />
    </Providers>,
  );
  await act(async () => {
    await retained?.();
    first.reject(new Error("first A"));
    second.resolve(success("B"));
  });
  expect(requestCommitDetail).toHaveBeenCalledTimes(3);
  expect(snapshot()).toMatchObject({ loading: true, commit: null, error: null });
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
  await act(async () => third.resolve(success("new A")));
  expect(snapshot().commit?.message).toBe("new A");
});

it("keeps StrictMode replay live without admitting the pre-cleanup request", async () => {
  const beforeCleanup = deferred();
  const replay = deferred();
  let retry: (() => Promise<void>) | undefined;
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(beforeCleanup.promise)
    .mockReturnValueOnce(replay.promise);
  render(
    <StrictMode>
      <Providers>
        <Reader
          onDetail={(detail) => {
            retry = detail.refetch;
          }}
        />
      </Providers>
    </StrictMode>,
  );
  expect(requestCommitDetail).toHaveBeenCalledTimes(2);
  await act(async () => beforeCleanup.reject(new Error("pre-cleanup")));
  expect(snapshot().loading).toBe(true);
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
  await act(async () => replay.resolve(success("replay")));
  expect(snapshot()).toMatchObject({ loading: false, commit: { message: "replay" } });
  vi.mocked(requestCommitDetail).mockResolvedValueOnce(success("retry"));
  await act(async () => {
    await retry?.();
  });
  expect(snapshot().commit?.message).toBe("retry");
  expect(requestCommitDetail).toHaveBeenCalledTimes(3);
});

it("retires one instance while another instance retains current failure feedback", async () => {
  const left = deferred();
  const right = deferred();
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(left.promise)
    .mockReturnValueOnce(right.promise);
  const view = render(
    <Providers>
      <Reader key="left" id="left" />
      <Reader key="right" id="right" />
    </Providers>,
  );
  view.rerender(
    <Providers>
      {false}
      <Reader key="right" id="right" />
    </Providers>,
  );
  await act(async () => {
    left.reject(new Error("retired left"));
    right.reject(new Error("current right"));
  });
  expect(screen.queryByTestId("left")).toBeNull();
  expect(snapshot("right")).toMatchObject({ loading: false, error: "current right" });
  expect(screen.getAllByTestId(toastTestId)).toHaveLength(1);
  expect(screen.getByTestId(toastTestId).textContent).toContain("current right");
  expect(scheduleFrontendErrorReport).toHaveBeenCalledTimes(1);
});

it.each(["success", "failure"])(
  "preserves loading and latest success across older %s",
  async (outcome) => {
    const first = deferred();
    const latest = deferred();
    let retry: (() => Promise<void>) | undefined;
    vi.mocked(requestCommitDetail)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(latest.promise);
    render(
      <Providers>
        <Reader
          onDetail={(detail) => {
            retry = detail.refetch;
          }}
        />
      </Providers>,
    );
    let completion: Promise<void> | undefined;
    act(() => {
      completion = retry?.();
    });
    expect(requestCommitDetail).toHaveBeenCalledTimes(2);
    await act(async () => {
      if (outcome === "success") first.resolve(success("older"));
      else first.reject(new Error("older"));
    });
    expect(snapshot()).toMatchObject({ loading: true, files: null, error: null });
    expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
    await act(async () => {
      latest.resolve(success("latest"));
      await completion;
    });
    expect(snapshot()).toMatchObject({
      loading: false,
      error: null,
      commit: { message: "latest" },
    });
  },
);

it("keeps latest failure when an older request succeeds later", async () => {
  const first = deferred();
  const latest = deferred();
  let retry: (() => Promise<void>) | undefined;
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(latest.promise);
  render(
    <Providers>
      <Reader
        onDetail={(detail) => {
          retry = detail.refetch;
        }}
      />
    </Providers>,
  );
  let completion: Promise<void> | undefined;
  act(() => {
    completion = retry?.();
  });
  await act(async () => {
    latest.reject(new Error("latest failure"));
    await completion;
  });
  await act(async () => first.resolve(success("older")));
  expect(snapshot()).toMatchObject({
    loading: false,
    files: null,
    commit: null,
    error: "latest failure",
  });
  expect(scheduleFrontendErrorReport).toHaveBeenCalledTimes(1);
});

// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.8
it.each(["rejection", "unsuccessful"])(
  "keeps current protocol %s localized and permits retry",
  async (outcome) => {
    if (outcome === "rejection") {
      vi.mocked(requestCommitDetail).mockRejectedValueOnce(
        new CommitDetailProtocolError("invalid_response"),
      );
    } else {
      vi.mocked(requestCommitDetail).mockResolvedValueOnce({ source: "github", success: false });
    }
    let retry: (() => Promise<void>) | undefined;
    render(
      <Providers>
        <Reader
          onDetail={(detail) => {
            retry = detail.refetch;
          }}
        />
      </Providers>,
    );
    await act(async () => {});
    expect(snapshot()).toMatchObject({
      loading: false,
      error: "Unexpected response from the server",
    });
    expect(screen.getByTestId(toastTestId).textContent).toContain(
      "Unexpected response from the server",
    );
    expect(scheduleFrontendErrorReport).toHaveBeenCalledTimes(1);
    vi.mocked(requestCommitDetail).mockResolvedValueOnce(success("recovered"));
    await act(async () => {
      await retry?.();
    });
    expect(snapshot()).toMatchObject({
      loading: false,
      error: null,
      commit: { message: "recovered" },
    });
    expect(vi.mocked(requestCommitDetail).mock.calls.map(([request]) => request)).toEqual([
      { target },
      { target },
    ]);
  },
);

function localState(): HydrationState {
  return {
    tasks: { ...defaultState.tasks, activeSessionId: "session-1", activeTaskId: "fallback-task" },
    taskSessions: {
      ...defaultState.taskSessions,
      items: {
        "session-1": {
          id: sessionId("session-1"),
          task_id: taskId("session-task"),
          state: "WAITING_FOR_INPUT",
          started_at: "2026-10-03T12:00:00Z",
          updated_at: "2026-10-03T12:00:00Z",
        },
      },
    },
    sessionAgentctl: { itemsBySessionId: { "session-1": { status: "starting" } } },
  };
}

// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.1
// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.2
it("retains local session/task routing and retries when readiness changes", async () => {
  const starting = deferred();
  const ready = deferred();
  let store!: StoreApi<AppState>;
  let retry: (() => Promise<void>) | undefined;
  const selected: CommitDetailTarget = { source: "local", sha: "local", repo: "subrepo" };
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(starting.promise)
    .mockReturnValueOnce(ready.promise);
  render(
    <Providers
      initialState={localState()}
      captureStore={(value) => {
        store = value;
      }}
    >
      <Reader
        selected={selected}
        onDetail={(detail) => {
          retry = detail.refetch;
        }}
      />
    </Providers>,
  );
  const retained = retry;
  expect(requestCommitDetail).toHaveBeenLastCalledWith({
    target: selected,
    local: { sessionId: "session-1", taskId: "session-task", agentctlReady: false },
  });
  act(() => store.getState().setSessionAgentctlStatus("session-1", { status: "ready" }));
  expect(requestCommitDetail).toHaveBeenLastCalledWith({
    target: selected,
    local: { sessionId: "session-1", taskId: "session-task", agentctlReady: true },
  });
  await act(async () => {
    await retained?.();
    starting.reject(new Error("not ready"));
  });
  expect(requestCommitDetail).toHaveBeenCalledTimes(2);
  expect(snapshot().loading).toBe(true);
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
  await act(async () => ready.resolve({ source: "local", success: true, files: success().files }));
  expect(snapshot()).toMatchObject({ loading: false, error: null, commit: null });
  expect(snapshot().files?.["file.ts"].diff).toBe("current");
});

it("retires replaced local session context and retains the active-task fallback", async () => {
  const first = deferred();
  const next = deferred();
  let store!: StoreApi<AppState>;
  let retry: (() => Promise<void>) | undefined;
  const selected: CommitDetailTarget = { source: "local", sha: "local", repo: "subrepo" };
  vi.mocked(requestCommitDetail)
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(next.promise);
  render(
    <Providers
      initialState={localState()}
      captureStore={(value) => {
        store = value;
      }}
    >
      <Reader
        selected={selected}
        onDetail={(detail) => {
          retry = detail.refetch;
        }}
      />
    </Providers>,
  );
  const retained = retry;
  act(() => store.getState().setActiveSessionAuto("next-task", "session-2"));
  expect(requestCommitDetail).toHaveBeenLastCalledWith({
    target: selected,
    local: { sessionId: "session-2", taskId: "next-task", agentctlReady: false },
  });
  await act(async () => {
    await retained?.();
    first.reject(new Error("old session"));
  });
  expect(requestCommitDetail).toHaveBeenCalledTimes(2);
  expect(scheduleFrontendErrorReport).not.toHaveBeenCalled();
  await act(async () => next.resolve({ source: "local", success: true, files: {} }));
  expect(snapshot()).toMatchObject({ loading: false, error: null, files: {} });
});

// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.3
// @covers AC-UI-PR-ONLY-COMMIT-DETAILS-001.6
it("keeps GitHub requests independent from local session/readiness updates", async () => {
  let store!: StoreApi<AppState>;
  vi.mocked(requestCommitDetail).mockResolvedValueOnce(success("remote"));
  render(
    <Providers
      initialState={localState()}
      captureStore={(value) => {
        store = value;
      }}
    >
      <Reader />
    </Providers>,
  );
  await act(async () => {});
  act(() => {
    store.getState().setSessionAgentctlStatus("session-1", { status: "ready" });
    store.getState().setActiveSessionAuto("next-task", "session-2");
  });
  expect(requestCommitDetail).toHaveBeenCalledTimes(1);
  expect(requestCommitDetail).toHaveBeenCalledWith({ target });
  expect(snapshot()).toMatchObject({ loading: false, commit: { message: "remote" } });
});
