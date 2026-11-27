import { Component, type ReactNode } from "react";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { useCopilotStore } from "@/hooks/domains/coordinator/copilot-store";
import { CoordinatorCopilotResetBridge } from "./coordinator-copilot-reset-bridge";

const EMPTY = { open: false, chip: null, draft: "" };
const SEEDED = {
  open: true,
  chip: { id: "KAN-1", label: "KAN-1", ref: { kind: "stall" as const, id: "t-1" } },
  draft: "Why?",
};
const VIEWS = ["", "/", "/queue", "/queue/"];

class EffectBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  render() {
    return this.state.failed ? <div data-testid="effect-failed" /> : this.props.children;
  }
}

function location(pathname: string) {
  window.history.replaceState({}, "", pathname);
  window.dispatchEvent(new PopStateEvent("popstate"));
}

function seed(coordinatorId: string) {
  const store = useCopilotStore.getState();
  store.askAboutThis(coordinatorId, "KAN-1", { kind: "stall", id: "t-1" }, "Why?");
  store.markDraftsSwept(coordinatorId);
  expect(useCopilotStore.getState().getEntry(coordinatorId)).toEqual(SEEDED);
  expect(useCopilotStore.getState().coordinatorId).toBe(coordinatorId);
  expect(useCopilotStore.getState().draftsSwept).toBe(true);
}

function mountBridge() {
  render(
    <EffectBoundary>
      <CoordinatorCopilotResetBridge />
      <div data-testid="mounted-application" />
    </EffectBoundary>,
  );
}

function expectMounted() {
  expect(screen.queryByTestId("effect-failed")).toBeNull();
  expect(screen.queryByTestId("mounted-application")).not.toBeNull();
}

function expectReset() {
  expectMounted();
  const store = useCopilotStore.getState();
  expect(store.coordinatorId).toBeNull();
  expect(store.getEntry("co-1")).toEqual(EMPTY);
  expect(store.draftsSwept).toBe(false);
}

let originalUrl: string;
let originalState: unknown;

beforeEach(() => {
  originalUrl = window.location.href;
  originalState = window.history.state;
  useCopilotStore.getState().keepOnlyFor(null);
  location("/workspaces/ws-1/coordinator/co-1");
});

afterEach(() => {
  cleanup();
  useCopilotStore.getState().keepOnlyFor(null);
  window.history.replaceState(originalState, "", originalUrl);
});

// @covers AC-COORDINATOR-COPILOT-004.12
describe("CoordinatorCopilotResetBridge with native pathname and real store", () => {
  it.each(["%", "ws%ZZ", "%E0%A4%A"])(
    "clears malformed workspace %s without replacing the mounted application",
    (workspace) => {
      mountBridge();
      for (const view of VIEWS) {
        act(() => location("/workspaces/ws-1/coordinator/co-1"));
        act(() => seed("co-1"));
        act(() => location(`/workspaces/${workspace}/coordinator/co-1${view}`));
        expectReset();
      }
    },
  );

  it.each(["%", "co%ZZ", "%E0%A4%A"])(
    "clears malformed coordinator %s without replacing the mounted application",
    (coordinator) => {
      // Each suffix starts with an independently seeded slot and mounted boundary.
      for (const view of VIEWS) {
        location("/workspaces/ws-1/coordinator/co-1");
        seed("co-1");
        mountBridge();
        act(() => location(`/workspaces/ws-1/coordinator/${coordinator}${view}`));
        expectReset();
        cleanup();
      }
    },
  );

  it.each([
    { workspace: "ws-1", coordinator: "co-1", id: "co-1" },
    { workspace: "ws%2Fone", coordinator: "co%2F1", id: "co/1" },
    { workspace: "ws%25", coordinator: "co%25", id: "co%" },
    { workspace: "ws%252Fone", coordinator: "co%252F1", id: "co%2F1" },
    { workspace: "ws%E6%97%A5", coordinator: "co%E6%97%A5", id: "co日" },
  ])("preserves the whole slot across native view transitions for $coordinator", (identity) => {
    const base = `/workspaces/${identity.workspace}/coordinator/${identity.coordinator}`;
    location(base);
    seed(identity.id);
    mountBridge();
    for (const view of ["/queue", "/queue/", "/", ""]) {
      act(() => location(`${base}${view}`));
      expectMounted();
      const store = useCopilotStore.getState();
      expect(store.coordinatorId).toBe(identity.id);
      expect(store.getEntry(identity.id)).toEqual(SEEDED);
      expect(store.draftsSwept).toBe(true);
    }
  });

  it.each([
    "/workspaces/ws-1/coordinator/co-2",
    "/",
    "/workspaces/ws-1/tasks",
    "/workspaces/ws-1/coordinator",
    "/workspaces/ws-1/coordinator/",
    "/workspaces//coordinator/co-1",
    "/workspaces/ws-1/coordinator//queue",
    "/workspaces/ws-1/coordinator/co-1/settings",
    "/workspaces/ws-1/coordinator/co-1/queue/extra",
  ])("clears each independent departure to %s", (pathname) => {
    seed("co-1");
    mountBridge();
    act(() => location(pathname));
    expectReset();
  });

  it("keeps chip and draft across close and reopen on the same recognized path", () => {
    seed("co-1");
    mountBridge();
    act(() => useCopilotStore.getState().setOpen("co-1", false));
    expect(useCopilotStore.getState().getEntry("co-1")).toEqual({ ...SEEDED, open: false });
    act(() => location("/workspaces/ws-1/coordinator/co-1/queue"));
    expect(useCopilotStore.getState().getEntry("co-1")).toEqual({ ...SEEDED, open: false });
    act(() => useCopilotStore.getState().setOpen("co-1", true));
    expectMounted();
    expect(useCopilotStore.getState().getEntry("co-1")).toEqual(SEEDED);
    expect(useCopilotStore.getState().draftsSwept).toBe(true);
  });
});
