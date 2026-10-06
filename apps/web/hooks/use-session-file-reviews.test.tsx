import { StrictMode, type PropsWithChildren } from "react";
import { act, cleanup, render, renderHook, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { ReviewDialog } from "@/components/review/review-dialog";
import { hashDiff, reviewFileKey, type ReviewFile } from "@/components/review/types";
import { computeChangesReviewSets } from "@/components/task/task-changes-panel-state";
import { defaultState } from "@/lib/state/default-state";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { useSessionFileReviews } from "./use-session-file-reviews";

type WireRequest = { id: string; action: string; payload: { session_id: string } };

// Only transport is replaced: protocol correlation, providers and consumers are real.
class DeferredSocket {
  static readonly OPEN = 1;
  static readonly CLOSED = 3;
  static current: DeferredSocket;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  requests: WireRequest[] = [];
  settled = new Set<string>();

  constructor() {
    DeferredSocket.current = this;
  }

  send(data: string) {
    this.requests.push(JSON.parse(data) as WireRequest);
  }

  close() {
    this.readyState = DeferredSocket.CLOSED;
  }

  settle(request: WireRequest, payload: unknown, type = "response") {
    expect(this.settled.has(request.id)).toBe(false);
    this.settled.add(request.id);
    this.onmessage?.({ data: JSON.stringify({ id: request.id, type, payload }) });
  }
}

const file: ReviewFile = {
  path: "same.ts",
  diff: "@@ -1 +1 @@\n-before\n+after",
  diff_state: "ready",
  status: "modified",
  additions: 1,
  deletions: 1,
  staged: false,
  source: "uncommitted",
};
const key = reviewFileKey(file);
let client: WebSocketClient;
let previousClient: WebSocketClient | null;
let sequence = 0;
const session = () => `file-review-reader-${++sequence}`;

function Providers({ children }: PropsWithChildren) {
  return (
    <StateProvider
      initialState={{
        userSettings: { ...defaultState.userSettings, reviewAutoMarkOnScroll: false },
      }}
    >
      <ToastProvider>
        <TooltipProvider>{children}</TooltipProvider>
      </ToastProvider>
    </StateProvider>
  );
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "queueMicrotask"] });
  previousClient = getWebSocketClient();
  vi.stubGlobal("WebSocket", DeferredSocket);
  client = new WebSocketClient("ws://file-review-reader-test", undefined, { enabled: false });
  client.connect();
  DeferredSocket.current.readyState = DeferredSocket.OPEN;
  DeferredSocket.current.onopen?.();
  setWebSocketClient(client);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    const wire = DeferredSocket.current;
    for (const request of wire.requests) {
      if (!wire.settled.has(request.id)) wire.settle(request, { reviews: [], success: true });
    }
    vi.runAllTicks();
  });
  client.disconnect();
  setWebSocketClient(previousClient);
  vi.clearAllTimers();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

async function flush() {
  await act(async () => {
    vi.runAllTicks();
    await vi.advanceTimersByTimeAsync(0);
  });
}

function requestFor(id: string, action = "session.file_review.get") {
  const matches = DeferredSocket.current.requests.filter(
    (request) => request.action === action && request.payload.session_id === id,
  );
  expect(matches).toHaveLength(1);
  return matches[0];
}

async function reply(id: string, reviewed: boolean, diffHash = hashDiff(file.diff)) {
  await act(async () => {
    DeferredSocket.current.settle(requestFor(id), {
      reviews: [
        {
          id: `${id}-row`,
          session_id: id,
          file_path: key,
          reviewed,
          diff_hash: diffHash,
          reviewed_at: null,
          created_at: "2026-10-06T00:00:00Z",
          updated_at: "2026-10-06T00:00:00Z",
        },
      ],
    });
  });
}

function reader(id: string | null) {
  return renderHook(({ selected }) => useSessionFileReviews(selected), {
    initialProps: { selected: id },
    wrapper: Providers,
  });
}

function reviewed(reviews: Map<string, { reviewed: boolean; diffHash: string }>) {
  return computeChangesReviewSets([file], reviews).reviewedFiles;
}

function registerSessionChangeTests() {
  // @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.12
  it("keeps Beta unreviewed after a late Alpha response with the same file and hash", async () => {
    const alpha = session(),
      beta = session();
    const { result, rerender } = reader(alpha);
    await flush();
    rerender({ selected: beta });
    await flush();
    await reply(beta, false);
    expect(reviewed(result.current.reviews)).toEqual(new Set());
    await reply(alpha, true);
    expect(reviewed(result.current.reviews)).toEqual(new Set());
  });

  // @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.13
  it("clears a completed map immediately when the committed session becomes null", async () => {
    const alpha = session();
    const { result, rerender } = reader(alpha);
    await flush();
    await reply(alpha, true);
    rerender({ selected: null });
    expect(result.current.reviews.size).toBe(0);
    expect(result.current.loading).toBe(false);
    await flush();
    expect(result.current.reviews.size).toBe(0);
  });

  it.each(["response", "error"])(
    "keeps Beta loading during retired Alpha %s settlement",
    async (type) => {
      const alpha = session(),
        beta = session();
      const { result, rerender } = reader(alpha);
      await flush();
      rerender({ selected: beta });
      await flush();
      await act(async () =>
        DeferredSocket.current.settle(requestFor(alpha), { reviews: [] }, type),
      );
      expect(result.current.loading).toBe(true);
      await reply(beta, true);
      expect(result.current.loading).toBe(false);
      expect(reviewed(result.current.reviews)).toEqual(new Set([key]));
    },
  );
}

function registerDeferredTests() {
  it("retires queued loading when switched to null before microtasks run", async () => {
    const { result, rerender } = reader(session());
    rerender({ selected: null });
    await flush();
    expect(result.current.loading).toBe(false);
    expect(result.current.reviews.size).toBe(0);
  });

  it("retires queued cache-hit publication before switching to an uncached session", async () => {
    const alpha = session(),
      beta = session();
    const seed = reader(alpha);
    await flush();
    await reply(alpha, true);
    seed.unmount();
    const { result, rerender } = reader(alpha);
    rerender({ selected: beta });
    await flush();
    expect(result.current.reviews.size).toBe(0);
    expect(result.current.loading).toBe(true);
    await reply(beta, false);
  });

  it("drops old loading on a completed cache hit and ignores other-session notifications", async () => {
    const alpha = session(),
      beta = session();
    const seed = reader(beta);
    await flush();
    await reply(beta, false);
    seed.unmount();
    const { result, rerender } = reader(alpha);
    await flush();
    rerender({ selected: beta });
    await flush();
    expect(result.current.loading).toBe(false);
    await reply(alpha, true);
    expect(reviewed(result.current.reviews)).toEqual(new Set());
    expect(result.current.loading).toBe(false);
  });
}

function registerCacheAndSharingTests() {
  // Shared cache publication is allowed even after the initiating reader retires.
  it("reuses a pending Alpha read after Alpha to Beta to Alpha without reviving its loading", async () => {
    const alpha = session(),
      beta = session();
    const { result, rerender } = reader(alpha);
    await flush();
    rerender({ selected: beta });
    await flush();
    rerender({ selected: alpha });
    await flush();
    expect(result.current.loading).toBe(false);
    requestFor(alpha);
    await reply(beta, false);
    expect(result.current.reviews.size).toBe(0);
    await reply(alpha, true);
    expect(reviewed(result.current.reviews)).toEqual(new Set([key]));
  });

  it("preserves proper-session cache settlement after initiator unmount and StrictMode replay", async () => {
    const alpha = session();
    const initial = renderHook(() => useSessionFileReviews(alpha), {
      wrapper: ({ children }) => (
        <StrictMode>
          <Providers>{children}</Providers>
        </StrictMode>
      ),
    });
    await flush();
    requestFor(alpha);
    initial.unmount();
    await reply(alpha, true);
    const next = reader(alpha);
    await flush();
    expect(reviewed(next.result.current.reviews)).toEqual(new Set([key]));
    expect(next.result.current.loading).toBe(false);
    requestFor(alpha);
  });

  // @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.14
  it("shares one read and reactive optimistic controls across same-session readers", async () => {
    const alpha = session();
    const one = reader(alpha),
      two = reader(alpha);
    await flush();
    await reply(alpha, true);
    expect(reviewed(one.result.current.reviews)).toEqual(new Set([key]));
    expect(reviewed(two.result.current.reviews)).toEqual(new Set([key]));
    act(() => one.result.current.markUnreviewed(key));
    expect(reviewed(two.result.current.reviews)).toEqual(new Set());
    await act(async () =>
      DeferredSocket.current.settle(requestFor(alpha, "session.file_review.update"), {
        success: true,
      }),
    );
    act(() => two.result.current.markReviewed(key, hashDiff(file.diff)));
    expect(reviewed(one.result.current.reviews)).toEqual(new Set([key]));
    act(() => one.result.current.resetReviews());
    expect(two.result.current.reviews.size).toBe(0);
  });

  it("preserves current optimistic changes before deferred cache-hit initialization", async () => {
    const alpha = session();
    const seed = reader(alpha);
    await flush();
    await reply(alpha, true);
    seed.unmount();
    const current = reader(alpha);
    act(() => current.result.current.markUnreviewed(key));
    await flush();
    expect(reviewed(current.result.current.reviews)).toEqual(new Set());
  });
}

function registerCurrentControls() {
  it("preserves current mark rollback while suppressing its retired direct publisher", async () => {
    const alpha = session(),
      beta = session();
    const { result, rerender } = reader(alpha);
    await flush();
    await reply(alpha, false);
    act(() => result.current.markReviewed(key, hashDiff(file.diff)));
    await act(async () =>
      DeferredSocket.current.settle(
        requestFor(alpha, "session.file_review.update"),
        { code: "failed", message: "Failed" },
        "error",
      ),
    );
    expect(result.current.reviews.has(key)).toBe(false);
    act(() => result.current.markReviewed(key, hashDiff(file.diff)));
    rerender({ selected: beta });
    await flush();
    await reply(beta, true);
    const pending = DeferredSocket.current.requests
      .filter((request) => request.action === "session.file_review.update")
      .at(-1)!;
    await act(async () =>
      DeferredSocket.current.settle(pending, { code: "failed", message: "Failed" }, "error"),
    );
    expect(reviewed(result.current.reviews)).toEqual(new Set([key]));
  });

  it.each(["empty", "error", "no-client"])(
    "settles current %s reads idle without reviewed state",
    async (outcome) => {
      const alpha = session();
      if (outcome === "no-client") setWebSocketClient(null);
      const { result } = reader(alpha);
      await flush();
      if (outcome !== "no-client") {
        expect(result.current.loading).toBe(true);
        await act(async () =>
          DeferredSocket.current.settle(
            requestFor(alpha),
            { reviews: [], code: "failed", message: "Failed" },
            outcome === "error" ? "error" : "response",
          ),
        );
      }
      expect(result.current.loading).toBe(false);
      expect(result.current.reviews.size).toBe(0);
    },
  );

  it("retains current success loading and actual stale hash classification across rerenders", async () => {
    const alpha = session();
    const { result, rerender } = reader(alpha);
    await flush();
    rerender({ selected: alpha });
    expect(result.current.loading).toBe(true);
    await reply(alpha, true, hashDiff("different diff"));
    expect(result.current.loading).toBe(false);
    expect(computeChangesReviewSets([file], result.current.reviews).staleFiles).toEqual(
      new Set([key]),
    );
    requestFor(alpha);
  });
}

function registerConsumerTest() {
  it("keeps the real mounted Review dialog checkbox owned by Beta after late Alpha", async () => {
    const alpha = session(),
      beta = session();
    const props = {
      open: true,
      onOpenChange: vi.fn(),
      onSendComments: vi.fn(),
      cumulativeDiff: null,
      gitStatusFiles: { [file.path]: { ...file } },
      useRepositoryKeys: false,
    };
    const mounted = render(
      <VcsDialogsProvider sessionId={alpha}>
        <ReviewDialog {...props} sessionId={alpha} />
      </VcsDialogsProvider>,
      { wrapper: Providers },
    );
    await flush();
    const dialog = screen.getByRole("dialog");
    mounted.rerender(
      <VcsDialogsProvider sessionId={beta}>
        <ReviewDialog {...props} sessionId={beta} />
      </VcsDialogsProvider>,
    );
    await flush();
    await reply(beta, false);
    const checkbox = () => within(screen.getByTestId("review-file-row")).getByRole("checkbox");
    expect(checkbox().getAttribute("data-state")).toBe("unchecked");
    await reply(alpha, true);
    expect(screen.getByRole("dialog")).toBe(dialog);
    expect(checkbox().getAttribute("data-state")).toBe("unchecked");
  });
}

describe("session file review reader ownership", () => {
  registerSessionChangeTests();
  registerDeferredTests();
  registerCacheAndSharingTests();
  registerCurrentControls();
  registerConsumerTest();
});
