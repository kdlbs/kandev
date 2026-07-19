import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MobileSessionsPicker } from "./mobile-sessions-section";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  activeSessionId: "session-a" as string | null,
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
      tasks: { activeSessionId: mocks.activeSessionId },
      agentProfiles: { items: mocks.agentProfiles },
      kanban: { tasks: [{ id: "task-1", primarySessionId: "session-a" }] },
      messages: { bySession: mocks.messagesBySession },
      setActiveSession: vi.fn(),
    }),
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => (
    <span data-testid={`agent-logo-${agentName}`} />
  ),
}));

vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({
    setPrimary: vi.fn(),
    stop: vi.fn(),
    resume: vi.fn(),
    remove: vi.fn(),
  }),
  isSessionStoppable: () => false,
  isSessionDeletable: () => false,
  isSessionResumable: () => false,
}));

const PILL_TESTID = "mobile-sessions-pill";
const ICON_CIRCLE_CHECK = "tabler-icon-circle-check";
const SESSION_A = "session-a";
const SESSION_BG = "session-bg";
const START_TIME = "2026-01-01T00:00:00Z";

function session(
  id: string,
  profileId: string,
  startedAt: string,
  overrides: Partial<TaskSession> = {},
): TaskSession {
  return {
    id,
    task_id: "task-1",
    agent_profile_id: profileId,
    state: "WAITING_FOR_INPUT",
    started_at: startedAt,
    updated_at: startedAt,
    ...overrides,
  } as TaskSession;
}

function profile(id: string, label: string, agentName: string): AgentProfileOption {
  return {
    id,
    label: `Mock Agent • ${label}`,
    agent_id: `agent-${agentName}`,
    agent_name: agentName,
    cli_passthrough: false,
  };
}

afterEach(cleanup);

beforeEach(() => {
  mocks.activeSessionId = SESSION_A;
  mocks.sessions = [
    session(SESSION_A, "profile-a", START_TIME),
    session("session-b", "profile-b", "2026-01-01T00:01:00Z"),
  ];
  mocks.agentProfiles = [
    profile("profile-a", "Alpha", "claude"),
    profile("profile-b", "Beta", "codex"),
  ];
});

describe("MobileSessionsPicker", () => {
  it("uses the effective layout session instead of a stale store session", () => {
    render(<MobileSessionsPicker taskId="task-1" sessionId="session-b" fullWidth />);

    expect(
      screen.getByRole("button", { name: "Active session: Beta. Tap to switch." }),
    ).toBeTruthy();

    fireEvent.click(screen.getByTestId(PILL_TESTID));
    expect(screen.getByTestId("mobile-session-row-session-a").getAttribute("aria-current")).toBe(
      null,
    );
    expect(screen.getByTestId("mobile-session-row-session-b").getAttribute("aria-current")).toBe(
      "true",
    );
  });

  it("shows the effective session agent icon beside its label", () => {
    mocks.activeSessionId = "session-b";
    render(<MobileSessionsPicker taskId="task-1" sessionId="session-b" fullWidth />);

    const pill = screen.getByTestId(PILL_TESTID);
    expect(within(pill).getByTestId("mobile-session-agent-icon")).toBeTruthy();
    expect(within(pill).getByTestId("agent-logo-codex")).toBeTruthy();
  });

  it("renders background-running distinctly — matching desktop, not a done check", () => {
    // §spec:session-level-truth / §spec:state-vocabulary: a session whose
    // foreground turn is idle while spawned background work runs (RUNNING +
    // `background`) must read as background-running on mobile too — the shared
    // getSessionStateIcon spinner — distinct from generating and never a done
    // check. Tabler renders the icon shape into the svg class
    // (`tabler-icon-<name>`), so asserting the class proves the distinction is
    // carried by SHAPE (survives a grayscale scan), not hue alone.
    mocks.activeSessionId = SESSION_BG;
    mocks.sessions = [
      session(SESSION_BG, "profile-a", START_TIME, {
        state: "RUNNING",
        foreground_activity: "background",
      }),
      session("session-gen", "profile-b", "2026-01-01T00:01:00Z", {
        state: "RUNNING",
        foreground_activity: "generating",
      }),
      session("session-done", "profile-a", "2026-01-01T00:02:00Z", {
        state: "COMPLETED",
      }),
    ];
    render(<MobileSessionsPicker taskId="task-1" sessionId={SESSION_BG} fullWidth />);
    fireEvent.click(screen.getByTestId(PILL_TESTID));

    const bg = screen.getByTestId("mobile-session-state-session-bg");
    const gen = screen.getByTestId("mobile-session-state-session-gen");
    const done = screen.getByTestId("mobile-session-state-session-done");
    const svgClass = (el: HTMLElement) => el.querySelector("svg")?.getAttribute("class") ?? "";

    // background-running: the shared spinner, in motion, and a label that says so.
    expect(svgClass(bg)).toContain("tabler-icon-loader-2");
    expect(svgClass(bg)).toContain("animate-spin");
    expect(bg.textContent).toMatch(/background/i);

    // Distinct from generating: a static solid dot, no spin — a different SHAPE,
    // so the two read apart even desaturated.
    expect(svgClass(gen)).toContain("tabler-icon-circle-filled");
    expect(svgClass(gen)).not.toContain("animate-spin");
    expect(svgClass(bg)).not.toContain("tabler-icon-circle-filled");

    // Never a done check: distinct from a finished session, which shows the check.
    expect(svgClass(bg)).not.toContain(ICON_CIRCLE_CHECK);
    expect(svgClass(done)).toContain(ICON_CIRCLE_CHECK);
  });

  it("keeps the background-running label while the foreground has a pending prompt", () => {
    mocks.activeSessionId = SESSION_BG;
    mocks.sessions = [
      session(SESSION_BG, "profile-a", START_TIME, {
        state: "RUNNING",
        foreground_activity: "background",
      }),
    ];
    mocks.messagesBySession = {
      [SESSION_BG]: [{ type: "clarification_request", metadata: { status: "pending" } }],
    };

    render(<MobileSessionsPicker taskId="task-1" sessionId={SESSION_BG} fullWidth />);
    fireEvent.click(screen.getByTestId(PILL_TESTID));

    expect(screen.getByTestId("mobile-session-state-session-bg").textContent).toMatch(
      /background/i,
    );
    mocks.messagesBySession = {};
  });

  it("carries the waiting-for-input variants (§spec:waiting-for-input-parity)", () => {
    // A pending clarification and a pending permission each read distinctly on
    // the mobile session row — the question / shield glyphs — never a done check
    // or a running dot, matching the sidebar and desktop menus.
    mocks.activeSessionId = "session-clar";
    mocks.sessions = [
      session("session-clar", "profile-a", START_TIME, {
        state: "WAITING_FOR_INPUT",
      }),
      session("session-perm", "profile-b", "2026-01-01T00:01:00Z", {
        state: "WAITING_FOR_INPUT",
      }),
    ];
    mocks.messagesBySession = {
      "session-clar": [{ type: "clarification_request", metadata: { status: "pending" } }],
      "session-perm": [{ type: "permission_request", metadata: { status: "pending" } }],
    };
    render(<MobileSessionsPicker taskId="task-1" sessionId="session-clar" fullWidth />);
    fireEvent.click(screen.getByTestId(PILL_TESTID));

    const clar = screen.getByTestId("mobile-session-state-session-clar");
    const perm = screen.getByTestId("mobile-session-state-session-perm");
    const svgClass = (el: HTMLElement) => el.querySelector("svg")?.getAttribute("class") ?? "";

    expect(svgClass(clar)).toContain("tabler-icon-message-question");
    expect(svgClass(clar)).not.toContain(ICON_CIRCLE_CHECK);
    expect(svgClass(perm)).toContain("tabler-icon-shield-question");
    expect(svgClass(perm)).not.toContain(ICON_CIRCLE_CHECK);
    expect(perm.textContent).toMatch(/permission/i);

    mocks.messagesBySession = {};
  });
});
