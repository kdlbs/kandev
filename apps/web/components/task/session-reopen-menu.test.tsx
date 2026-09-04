import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent } from "@kandev/ui/dropdown-menu";
import { SessionReopenMenuItems, shouldShowReopenStateIcon } from "./session-reopen-menu";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";

describe("shouldShowReopenStateIcon", () => {
  it("surfaces the icon for a background-running session (RUNNING + background)", () => {
    // The defect this fixes: a session whose foreground turn is idle while
    // background work runs previously showed no icon (state dropped). It must
    // now render — the shared background-running spinner, never a done check.
    expect(shouldShowReopenStateIcon("RUNNING", "background")).toBe(true);
  });

  it("keeps a generating RUNNING session icon-less (established silent affordance)", () => {
    expect(shouldShowReopenStateIcon("RUNNING", "generating")).toBe(false);
  });

  it("falls back to silence — not done — when a RUNNING substate is unknown", () => {
    // §req safe-defaults: an unknown substate on a live session must never
    // resolve to a done affordance. Silence (no icon) is the safe reading here.
    expect(shouldShowReopenStateIcon("RUNNING", null)).toBe(false);
    expect(shouldShowReopenStateIcon("RUNNING", undefined)).toBe(false);
  });

  it("keeps STARTING icon-less (still launching)", () => {
    expect(shouldShowReopenStateIcon("STARTING", null)).toBe(false);
  });

  it("ignores stale pending input while a session is still STARTING", () => {
    expect(shouldShowReopenStateIcon("STARTING", null, true, false)).toBe(false);
    expect(shouldShowReopenStateIcon("STARTING", null, false, true)).toBe(false);
  });

  it("keeps a plain waiting session icon-less when no input is pending", () => {
    // WAITING_FOR_INPUT also means an ordinary turn finished and the session is
    // ready for another prompt; it is not proof that the agent asked a question.
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", null)).toBe(false);
  });

  it("surfaces explicit pending input while waiting", () => {
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", null, true, false)).toBe(true);
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", null, false, true)).toBe(true);
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", "background")).toBe(true);
  });

  it("surfaces the icon for a pending prompt even mid-turn (still coarsely RUNNING)", () => {
    // A pending clarification / permission is actionable; it must not be masked
    // by the generating-RUNNING silence rule.
    expect(shouldShowReopenStateIcon("RUNNING", "generating", true, false)).toBe(true);
    expect(shouldShowReopenStateIcon("RUNNING", "generating", false, true)).toBe(true);
  });

  it("renders the existing icon for terminal / other states", () => {
    expect(shouldShowReopenStateIcon("COMPLETED", null)).toBe(true);
    expect(shouldShowReopenStateIcon("FAILED", null)).toBe(true);
    expect(shouldShowReopenStateIcon("CANCELLED", null)).toBe(true);
    expect(shouldShowReopenStateIcon("CREATED", null)).toBe(true);
  });

  it("surfaces the icon for a parked-on-background-work session (AC-51/52)", () => {
    // Without this, a parked session with no live foregroundActivity would
    // render no icon at all: canRequestInput is true but every earlier
    // condition falls through to the WAITING_FOR_INPUT-no-pending silence rule.
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", null, false, false, true)).toBe(true);
    expect(shouldShowReopenStateIcon("RUNNING", null, false, false, true)).toBe(true);
  });

  it("lets a pending prompt outrank the parked reading", () => {
    expect(shouldShowReopenStateIcon("WAITING_FOR_INPUT", null, true, false, true)).toBe(true);
  });

  it("ignores a parked reading while a session is still STARTING", () => {
    expect(shouldShowReopenStateIcon("STARTING", null, false, false, true)).toBe(false);
  });
});

const mocks = vi.hoisted(() => ({
  sessions: [] as TaskSession[],
  agentProfiles: [] as AgentProfileOption[],
  messagesBySession: {} as Record<string, unknown[]>,
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ sessions: mocks.sessions, isLoading: false, isLoaded: true }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: mocks.agentProfiles },
      kanban: { tasks: [{ id: "task-1", primarySessionId: null }] },
      messages: { bySession: mocks.messagesBySession },
    }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: unknown) => unknown) =>
    selector({ api: null, centerGroupId: "center" }),
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: () => null,
}));

const TASK_ID = "task-1";
const START_TIME = "2026-01-01T00:00:00Z";

function session(id: string, overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id,
    task_id: TASK_ID,
    agent_profile_id: "profile-a",
    state: "WAITING_FOR_INPUT",
    started_at: START_TIME,
    updated_at: START_TIME,
    ...overrides,
  } as TaskSession;
}

function renderMenu() {
  return render(
    <DropdownMenu defaultOpen modal={false}>
      <DropdownMenuContent forceMount>
        <SessionReopenMenuItems taskId={TASK_ID} />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

afterEach(cleanup);

beforeEach(() => {
  mocks.sessions = [];
  mocks.agentProfiles = [
    {
      id: "profile-a",
      label: "Mock Agent • Alpha",
      agent_id: "agent-claude",
      agent_name: "claude",
    },
  ] as AgentProfileOption[];
  mocks.messagesBySession = {};
});

// AC-51/52: the menu row's icon is wired through shouldShowReopenStateIcon AND
// getSessionStateIcon's own precedence table. A render-level test — not just
// the boolean predicate above — is what actually proves the two stay wired
// together (e.g. a prop-name typo passing parked_on_background_work through
// would still pass the pure predicate tests, but produce no icon here).
describe("SessionReopenMenuItems icon rendering (AC-51/52)", () => {
  it("renders the shared background spinner for a parked-on-background-work session", () => {
    mocks.sessions = [
      session("sess-parked", { parked_on_background_work: true } as Partial<TaskSession>),
    ];

    renderMenu();

    const row = screen.getByTestId("reopen-session-sess-parked");
    const svgClass = row.querySelector("svg")?.getAttribute("class") ?? "";
    expect(svgClass).toContain("tabler-icon-loader-2");
    expect(svgClass).toContain("animate-spin");
  });

  it("renders no state icon for a plain waiting session with nothing pending", () => {
    mocks.sessions = [session("sess-plain")];

    renderMenu();

    const row = screen.getByTestId("reopen-session-sess-plain");
    expect(row.querySelector("svg")).toBeNull();
  });

  it("lets a pending clarification outrank the parked reading in the rendered icon", () => {
    mocks.sessions = [
      session("sess-parked-pending", { parked_on_background_work: true } as Partial<TaskSession>),
    ];
    mocks.messagesBySession = {
      "sess-parked-pending": [{ type: "clarification_request", metadata: { status: "pending" } }],
    };

    renderMenu();

    const row = screen.getByTestId("reopen-session-sess-parked-pending");
    const svgClass = row.querySelector("svg")?.getAttribute("class") ?? "";
    expect(svgClass).toContain("tabler-icon-message-question");
    expect(svgClass).not.toContain("tabler-icon-loader-2");
  });
});
