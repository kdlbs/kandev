import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Relay, RelayRead } from "@/lib/api/domains/coordinator-relay-api";
import type { Message } from "@/lib/types/http";
import { useRelayItem } from "./use-relay-item";

const readRelay = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/coordinator-relay-api", () => ({ readRelay }));

type Deferred = { resolve: (value: RelayRead) => void };
let pending: Deferred[] = [];

function deferredRead(): Promise<RelayRead> {
  return new Promise((resolve) => pending.push({ resolve }));
}

function bundle(pendingId: string): Relay {
  return {
    task_id: "t1",
    session_id: "s1",
    clarification: { pending_id: pendingId, context: "", messages: [] },
    permission: null,
  };
}

function permission(requestId: string): Relay {
  const message = {
    id: "m",
    metadata: { request_id: requestId, pending_id: "p" },
  } as unknown as Message;
  return { task_id: "t1", session_id: "s1", clarification: null, permission: { message } };
}

const NONE: Relay = { task_id: "t1", session_id: "s1", clarification: null, permission: null };
const ok = (relay: Relay): RelayRead => ({ kind: "ok", relay });

type Props = {
  enabled?: boolean;
  pendingAction?: "clarification" | "permission" | undefined;
  refreshKey?: string;
};

function setup(initial: Props = {}) {
  return renderHook(
    (props: Props) =>
      useRelayItem({
        workspaceId: "ws",
        coordinatorId: "co",
        taskId: "t1",
        pendingAction: "pendingAction" in props ? props.pendingAction : "clarification",
        enabled: props.enabled ?? true,
        refreshKey: props.refreshKey ?? "k0",
      }),
    { initialProps: initial },
  );
}

async function settle(index: number, value: RelayRead) {
  await act(async () => {
    pending[index].resolve(value);
  });
}

beforeEach(() => {
  pending = [];
  readRelay.mockReset();
  readRelay.mockImplementation(() => deferredRead());
});

async function expandedClarification(pendingId = "b1") {
  const hook = setup();
  await settle(0, ok(bundle(pendingId)));
  act(() => hook.result.current.toggle());
  return hook;
}

describe("useRelayItem availability", () => {
  it("makes no read at all when not enabled, including on an outcome collapse", async () => {
    const hook = setup({ enabled: false });
    expect(readRelay).not.toHaveBeenCalled();
    act(() => hook.result.current.finishOutcome());
    expect(readRelay).not.toHaveBeenCalled();
    expect(hook.result.current.offered).toBe(false);
  });

  it("offers Answer here only once the read carries an answerable item of the item's kind", async () => {
    const hook = setup({ pendingAction: "permission" });
    expect(hook.result.current.offered).toBe(false);
    await settle(0, ok(bundle("b1")));
    expect(hook.result.current.offered).toBe(false);
    hook.rerender({ pendingAction: "permission", refreshKey: "k1" });
    await settle(1, ok(permission("r1")));
    expect(hook.result.current.offered).toBe(true);
  });

  it("stops offering when a later event read is null or fails", async () => {
    const hook = setup();
    await settle(0, ok(bundle("b1")));
    expect(hook.result.current.offered).toBe(true);
    hook.rerender({ refreshKey: "k1" });
    await settle(1, { kind: "failed" });
    expect(hook.result.current.offered).toBe(false);
  });

  it("drops a response for an older request", async () => {
    const hook = setup();
    hook.rerender({ refreshKey: "k1" });
    await settle(1, ok(bundle("new")));
    await settle(0, ok(NONE));
    expect(hook.result.current.offered).toBe(true);
  });
});

describe("useRelayItem collapse paths", () => {
  it("collapses on a not-engaged expand read that returns a null field", async () => {
    const hook = await expandedClarification();
    expect(hook.result.current.expanded).toBe(true);
    await settle(1, ok(NONE));
    expect(hook.result.current.expanded).toBe(false);
    expect(hook.result.current.offered).toBe(false);
  });

  it("collapses on a not-engaged expand read that fails", async () => {
    const hook = await expandedClarification();
    await settle(1, { kind: "failed" });
    expect(hook.result.current.expanded).toBe(false);
  });

  it("collapses on a refused expand read and stops offering", async () => {
    const hook = await expandedClarification();
    await settle(1, { kind: "refused" });
    expect(hook.result.current.expanded).toBe(false);
    expect(hook.result.current.offered).toBe(false);
  });

  it("collapses on a manual toggle and drops the in-flight expand read", async () => {
    const hook = await expandedClarification();
    act(() => hook.result.current.toggle());
    expect(hook.result.current.expanded).toBe(false);
    await settle(1, ok(bundle("b2")));
    expect(hook.result.current.expanded).toBe(false);
  });

  it("collapses on an outcome, discards the cache and issues one fresh read", async () => {
    const hook = await expandedClarification();
    await settle(1, ok(bundle("b1")));
    const before = readRelay.mock.calls.length;
    act(() => hook.result.current.finishOutcome());
    expect(hook.result.current.expanded).toBe(false);
    expect(hook.result.current.offered).toBe(false);
    expect(readRelay.mock.calls.length).toBe(before + 1);
    await settle(2, ok(bundle("b1")));
    expect(hook.result.current.offered).toBe(true);
  });
});

describe("useRelayItem expand and event reads", () => {
  it("replaces the bundle when the expand read returns a different pending id", async () => {
    const hook = await expandedClarification("b1");
    await settle(1, ok(bundle("b2")));
    const rendered = hook.result.current.rendered;
    expect(rendered?.kind === "clarification" && rendered.bundle.pending_id).toBe("b2");
  });

  it("keeps the rendered bundle when the pending id matches", async () => {
    const hook = await expandedClarification("b1");
    const first = hook.result.current.rendered;
    await settle(1, ok(bundle("b1")));
    expect(hook.result.current.rendered).toBe(first);
  });

  it("ignores every read once engaged, including the in-flight expand read", async () => {
    const hook = await expandedClarification("b1");
    act(() => hook.result.current.markEngaged());
    await settle(1, ok(NONE));
    expect(hook.result.current.expanded).toBe(true);
    hook.rerender({ refreshKey: "k1" });
    await settle(2, ok(bundle("b2")));
    const rendered = hook.result.current.rendered;
    expect(rendered?.kind === "clarification" && rendered.bundle.pending_id).toBe("b1");
  });

  it("issues no event read while the expand read is in flight and keeps its result", async () => {
    const hook = await expandedClarification("b1");
    const calls = readRelay.mock.calls.length;
    hook.rerender({ refreshKey: "k1" });
    expect(readRelay.mock.calls.length).toBe(calls);
    await settle(1, ok(bundle("b2")));
    const rendered = hook.result.current.rendered;
    expect(rendered?.kind === "clarification" && rendered.bundle.pending_id).toBe("b2");
  });

  it("never collapses on an event read, and replaces a permission only on a new request id", async () => {
    const hook = setup({ pendingAction: "permission" });
    await settle(0, ok(permission("r1")));
    act(() => hook.result.current.toggle());
    await settle(1, ok(permission("r1")));
    hook.rerender({ pendingAction: "permission", refreshKey: "k1" });
    await settle(2, ok(NONE));
    expect(hook.result.current.expanded).toBe(true);
    hook.rerender({ pendingAction: "permission", refreshKey: "k2" });
    await settle(3, { kind: "failed" });
    expect(hook.result.current.expanded).toBe(true);
    hook.rerender({ pendingAction: "permission", refreshKey: "k3" });
    await settle(4, ok(permission("r2")));
    const rendered = hook.result.current.rendered;
    expect(
      rendered?.kind === "permission" &&
        (rendered.permission.message.metadata as { request_id: string }).request_id,
    ).toBe("r2");
  });
});

describe("useRelayItem held expansion", () => {
  it("stays expanded with its kind when the pending action clears or the viewer stops being a manager", async () => {
    const hook = await expandedClarification("b1");
    await settle(1, ok(bundle("b1")));
    hook.rerender({ pendingAction: undefined, enabled: false });
    expect(hook.result.current.expanded).toBe(true);
    expect(hook.result.current.rendered?.kind).toBe("clarification");
  });
});
