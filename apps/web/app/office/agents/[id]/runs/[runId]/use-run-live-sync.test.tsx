import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { RunEvent } from "@/lib/api/domains/office-extended-api";
import type { RunEventAppendedPayload } from "@/lib/types/backend";
import { useRunLiveSync } from "./use-run-live-sync";

const api = vi.hoisted(() => ({
  getRunDetail: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-runs-api", () => api);

const clients = vi.hoisted(() => ({
  active: { subscribeRun: vi.fn() },
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => clients.active,
  useWebSocketClient: () => clients.active,
}));

const handlers = vi.hoisted(() => ({
  listener: undefined as ((payload: RunEventAppendedPayload) => void) | undefined,
}));

vi.mock("@/lib/ws/handlers/run", () => ({
  subscribeRunEvents: (_runId: string, listener: (payload: RunEventAppendedPayload) => void) => {
    handlers.listener = listener;
    return () => {
      handlers.listener = undefined;
    };
  },
}));

function runEvent(seq: number, eventType: string): RunEvent {
  return {
    seq,
    event_type: eventType,
    level: "info",
    payload: "{}",
    created_at: "2026-01-01T00:00:00Z",
  };
}

type Props = {
  runId: string;
  initialEvents: RunEvent[];
  initialStatus: "claimed";
};

function renderLiveSync(initialProps: Props) {
  return renderHook(
    (props: Props) => useRunLiveSync(props.runId, props.initialEvents, props.initialStatus),
    { initialProps },
  );
}

const initialProps: Props = { runId: "run-1", initialEvents: [], initialStatus: "claimed" };

describe("useRunLiveSync", () => {
  beforeEach(() => {
    clients.active = { subscribeRun: vi.fn(() => vi.fn()) };
    handlers.listener = undefined;
  });

  it("does not loop when the snapshot reference changes but its content does not", () => {
    // A fresh array literal on every render changes `initialEvents` identity
    // without changing its content. Without the content guard the snapshot
    // sync effect re-writes state on every render, feeding an endless
    // render->effect cycle that exhausts the worker (this test times out
    // rather than completing).
    const { result, rerender } = renderHook(() => useRunLiveSync("run-1", [], "claimed"));

    rerender();
    rerender();

    expect(clients.active.subscribeRun).toHaveBeenCalledOnce();
    expect(result.current.events).toEqual([]);
    expect(result.current.status).toBe("claimed");
  });

  it("does not clobber a live event when a fresh but unchanged snapshot rerenders", () => {
    // A caller that supplies a fresh-but-unchanged `initialEvents` array (the
    // exact scenario the snapshot guard exists for) must not erase events that
    // arrived over the WS. The snapshot sync decision has to compare against
    // the last incoming snapshot, not against the live-event dedup set: once
    // the first WS event adds its seq to the dedup set, comparing the stale
    // snapshot against that set looks like a content change and resets events
    // (and the dedup set) to the snapshot, making the live event vanish.
    const { result, rerender } = renderLiveSync(initialProps);

    const liveEvent = runEvent(1, "started");
    act(() => {
      handlers.listener!({ run_id: "run-1", event: liveEvent });
    });
    expect(result.current.events).toEqual([liveEvent]);

    rerender({ ...initialProps, initialEvents: [] });

    expect(result.current.events).toEqual([liveEvent]);
  });

  it("merges a changed snapshot without dropping a live event", () => {
    const initialEvent = runEvent(1, "init");
    const liveEvent = runEvent(2, "started");
    const snapshotEvent = runEvent(3, "step");
    const { result, rerender } = renderLiveSync({
      ...initialProps,
      initialEvents: [initialEvent],
    });

    act(() => {
      handlers.listener!({ run_id: "run-1", event: liveEvent });
    });
    rerender({ ...initialProps, initialEvents: [initialEvent, snapshotEvent] });

    expect(result.current.events).toEqual([initialEvent, liveEvent, snapshotEvent]);
  });

  it("re-syncs events when the snapshot content actually changes", () => {
    const started = runEvent(1, "started");
    const { result, rerender } = renderLiveSync(initialProps);

    rerender({ ...initialProps, initialEvents: [started] });

    expect(result.current.events).toEqual([started]);
  });

  it("reloads the final output summary after a terminal event", async () => {
    api.getRunDetail.mockResolvedValue({ output_summary: "final answer" });
    const { result } = renderHook(() =>
      useRunLiveSync("run-1", [], "claimed", { agentId: "agent-1" }),
    );

    act(() => {
      handlers.listener!({ run_id: "run-1", event: runEvent(1, "complete") });
    });

    await vi.waitFor(() => {
      expect(api.getRunDetail).toHaveBeenCalledWith("agent-1", "run-1");
      expect(result.current.outputSummary).toBe("final answer");
    });
  });
});
