import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import type { TaskSession } from "@/app/office/tasks/[id]/types";

// Provide a simple in-memory localStorage mock so the tests are not sensitive
// to how the test runner exposes window.localStorage.
function makeLocalStorageMock() {
  const store = new Map<string, string>();
  return {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => store.set(key, value),
    removeItem: (key: string) => store.delete(key),
    clear: () => store.clear(),
    get length() {
      return store.size;
    },
    key: (index: number) => Array.from(store.keys())[index] ?? null,
  };
}
const localStorageMock = makeLocalStorageMock();
vi.stubGlobal("localStorage", localStorageMock);

// Mock AdvancedChatPanel — its full hook tree is tested elsewhere and would
// require a WS client / agent-profile fixtures for this test to render. Here
// we only care about the header / collapse / ref-registration behavior.
vi.mock("@/app/office/tasks/[id]/advanced-panels/chat-panel", () => ({
  AdvancedChatPanel: ({ sessionId }: { sessionId: string | null }) => (
    <div data-testid="advanced-chat-panel-mock">embed:{sessionId ?? "none"}</div>
  ),
}));

// Stub Collapsible to avoid Radix portals + JSX-in-deps quirks under vitest.
// We propagate `open` from <Collapsible> via React Context so <CollapsibleContent>
// can hide itself when closed (matching Radix's behavior for our assertions).
import { createContext, useContext, useEffect } from "react";
const CollapsibleOpenCtx = createContext<{
  open: boolean;
  onOpenChange?: (next: boolean) => void;
}>({
  open: true,
});
vi.mock("@kandev/ui/collapsible", () => ({
  Collapsible: ({
    open,
    onOpenChange,
    children,
  }: {
    open?: boolean;
    onOpenChange?: (v: boolean) => void;
    children: React.ReactNode;
  }) => (
    <CollapsibleOpenCtx.Provider value={{ open: !!open, onOpenChange }}>
      <div data-open={open}>{children}</div>
    </CollapsibleOpenCtx.Provider>
  ),
  CollapsibleTrigger: ({
    children,
    className,
  }: {
    children: React.ReactNode;
    className?: string;
  }) => {
    const { open, onOpenChange } = useContext(CollapsibleOpenCtx);
    return (
      <button type="button" onClick={() => onOpenChange?.(!open)} className={className}>
        {children}
      </button>
    );
  },
  CollapsibleContent: ({ children }: { children: React.ReactNode }) => {
    const { open } = useContext(CollapsibleOpenCtx);
    if (!open) return null;
    return <div data-testid="collapsible-content">{children}</div>;
  },
}));

import { SessionTimelineEntry } from "./session-timeline-entry";
import { ActiveSessionRefProvider, useActiveSessionRef } from "./active-session-ref-context";

function makeSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: "sess-1",
    agentName: "Alice",
    agentRole: "agent",
    state: "RUNNING",
    isPrimary: true,
    startedAt: "2026-05-01T10:00:00Z",
    updatedAt: "2026-05-01T10:00:30Z",
    ...overrides,
  };
}

function wrap(node: ReactNode) {
  return (
    <StateProvider>
      <ActiveSessionRefProvider>{node}</ActiveSessionRefProvider>
    </StateProvider>
  );
}

beforeEach(() => {
  localStorageMock.clear();
});

afterEach(() => {
  cleanup();
});

async function renderAndCaptureActiveNode(
  state: TaskSession["state"],
): Promise<HTMLElement | null> {
  const probeRef: { current: HTMLElement | null } = { current: null };
  function Probe() {
    const { getActiveNode } = useActiveSessionRef();
    useEffect(() => {
      probeRef.current = getActiveNode();
    });
    return null;
  }
  render(
    wrap(
      <>
        <SessionTimelineEntry taskId="task-1" session={makeSession({ state })} />
        <Probe />
      </>,
    ),
  );
  await Promise.resolve();
  return probeRef.current;
}

const EMBED_TID = "advanced-chat-panel-mock";

describe("SessionTimelineEntry", () => {
  it("renders 'working' header for active sessions and is expanded by default", () => {
    render(
      wrap(<SessionTimelineEntry taskId="task-1" session={makeSession({ state: "RUNNING" })} />),
    );
    expect(screen.getByText("Alice")).toBeTruthy();
    expect(screen.getByText(/working/)).toBeTruthy();
    // Embedded panel visible -> expanded
    expect(screen.getByTestId(EMBED_TID)).toBeTruthy();
  });

  it("renders 'worked' header for terminal sessions and is collapsed by default", () => {
    render(
      wrap(<SessionTimelineEntry taskId="task-1" session={makeSession({ state: "COMPLETED" })} />),
    );
    expect(screen.getByText(/worked/)).toBeTruthy();
    // Collapsed -> embed not in DOM (Radix Collapsible removes content when closed).
    expect(screen.queryByTestId(EMBED_TID)).toBeNull();
  });

  it("persists collapse state per session in localStorage", () => {
    const session = makeSession({ id: "sess-persist", state: "RUNNING" });
    render(wrap(<SessionTimelineEntry taskId="task-1" session={session} />));
    // Click trigger to collapse
    const trigger = screen.getByRole("button", { name: /Alice/ });
    fireEvent.click(trigger);
    expect(localStorageMock.getItem("office.session.collapsed.sess-persist")).toBe("1");

    // Unmount + remount: the persisted "collapsed=true" wins over the active default.
    cleanup();
    render(wrap(<SessionTimelineEntry taskId="task-1" session={session} />));
    expect(screen.queryByTestId(EMBED_TID)).toBeNull();
  });

  it("registers its DOM node with ActiveSessionRefContext when live", async () => {
    const captured = await renderAndCaptureActiveNode("RUNNING");
    expect(captured).not.toBeNull();
  });

  it("does not register a terminal session as the active ref", async () => {
    const captured = await renderAndCaptureActiveNode("COMPLETED");
    expect(captured).toBeNull();
  });
});

describe("SessionTimelineEntry — office (multi-agent)", () => {
  it("renders 'worked' header for office IDLE sessions and is collapsed by default", () => {
    render(
      wrap(
        <SessionTimelineEntry
          taskId="task-1"
          session={makeSession({
            state: "IDLE",
            agentProfileId: "agent-a",
          })}
        />,
      ),
    );
    // IDLE renders "worked" — "idle for Xs" was confusing because Xs
    // is the turn duration, not "time spent doing nothing".
    expect(screen.getByText(/worked/)).toBeTruthy();
    // Collapsed by default: embed not rendered.
    expect(screen.queryByTestId(EMBED_TID)).toBeNull();
  });

  it("treats group as live when any session in groupSessions is RUNNING", () => {
    const idleRow = makeSession({
      id: "sess-old",
      state: "IDLE",
      agentProfileId: "agent-a",
      startedAt: "2026-05-01T10:00:00Z",
    });
    const runningRow = makeSession({
      id: "sess-new",
      state: "RUNNING",
      agentProfileId: "agent-a",
      startedAt: "2026-05-01T11:00:00Z",
    });
    render(
      wrap(
        <SessionTimelineEntry
          taskId="task-1"
          session={runningRow}
          groupSessions={[idleRow, runningRow]}
        />,
      ),
    );
    expect(screen.getByText(/working/)).toBeTruthy();
    expect(screen.getByTestId("session-state-running")).toBeTruthy();
    // Embed visible because expanded by default while live.
    expect(screen.getByTestId(EMBED_TID)).toBeTruthy();
  });

  it("renders the role chip when provided", () => {
    render(
      wrap(
        <SessionTimelineEntry
          taskId="task-1"
          session={makeSession({
            state: "IDLE",
            agentProfileId: "agent-rev",
          })}
          roleChip="Reviewer"
        />,
      ),
    );
    expect(screen.getByTestId("session-role-chip").textContent).toBe("Reviewer");
  });

  it("does NOT render a role chip when none is provided", () => {
    render(
      wrap(<SessionTimelineEntry taskId="task-1" session={makeSession({ state: "RUNNING" })} />),
    );
    expect(screen.queryByTestId("session-role-chip")).toBeNull();
  });
});
