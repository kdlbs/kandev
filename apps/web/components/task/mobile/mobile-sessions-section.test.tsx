import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MobileSessionsPicker } from "./mobile-sessions-section";
import type { AgentProfileOption } from "@/lib/state/slices";
import { repositoryId, type Repository, type TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  activeSessionId: "session-a" as string | null,
  sessions: [] as TaskSession[],
  agentProfiles: [] as AgentProfileOption[],
  repositoriesByWorkspaceId: {} as Record<string, Repository[]>,
  messagesBySession: {} as Record<string, unknown[]>,
  turnsBySession: {} as Record<string, unknown[]>,
  setActiveSession: vi.fn(),
  removeSession: vi.fn(),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ sessions: mocks.sessions, isLoading: false, isLoaded: true }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      features: { dynamicAgentRouting: false },
      tasks: { activeSessionId: mocks.activeSessionId },
      agentProfiles: { items: mocks.agentProfiles },
      kanban: { tasks: [{ id: "task-1", primarySessionId: "session-a" }] },
      repositories: { itemsByWorkspaceId: mocks.repositoriesByWorkspaceId },
      messages: { bySession: mocks.messagesBySession },
      turns: { bySession: mocks.turnsBySession },
      executors: { items: [] },
      setActiveSession: mocks.setActiveSession,
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
    remove: mocks.removeSession,
  }),
  isSessionStoppable: () => false,
  isSessionDeletable: () => true,
  isSessionResumable: () => false,
}));

const PILL_TESTID = "mobile-sessions-pill";
const ICON_CIRCLE_CHECK = "tabler-icon-circle-check";
const SESSION_ACTIONS_LABEL = "Session actions";
const SESSION_A = "session-a";
const SESSION_BG = "session-bg";
const TASK_ID = "task-1";
const START_TIME = "2026-01-01T00:00:00Z";
const SECOND_TIME = "2026-01-01T00:01:00Z";

function session(
  id: string,
  profileId: string,
  startedAt: string,
  overrides: Partial<TaskSession> = {},
): TaskSession {
  return {
    id,
    task_id: TASK_ID,
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

function repository(id: string, name: string): Repository {
  return {
    id,
    workspace_id: "workspace-1",
    name,
    source_type: "local",
    local_path: `/tmp/${name}`,
    provider: "",
    provider_repo_id: "",
    provider_owner: "",
    provider_name: "",
    default_branch: "main",
    worktree_branch_prefix: "kandev/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: START_TIME,
    updated_at: START_TIME,
  } as Repository;
}

afterEach(cleanup);

beforeEach(() => {
  mocks.activeSessionId = SESSION_A;
  mocks.sessions = [
    session(SESSION_A, "profile-a", START_TIME),
    session("session-b", "profile-b", SECOND_TIME),
  ];
  mocks.agentProfiles = [
    profile("profile-a", "Alpha", "claude"),
    profile("profile-b", "Beta", "codex"),
  ];
  mocks.repositoriesByWorkspaceId = {};
  mocks.messagesBySession = {};
  mocks.turnsBySession = {};
  mocks.setActiveSession.mockReset();
  mocks.removeSession.mockReset();
});

describe("MobileSessionsPicker selection", () => {
  it("uses the effective layout session instead of a stale store session", () => {
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId="session-b" fullWidth />);

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
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId="session-b" fullWidth />);

    const pill = screen.getByTestId(PILL_TESTID);
    expect(within(pill).getByTestId("mobile-session-agent-icon")).toBeTruthy();
    expect(within(pill).getByTestId("agent-logo-codex")).toBeTruthy();
  });

  it("disambiguates repository-bound sessions without a workflow task snapshot", () => {
    mocks.sessions = [
      session(SESSION_A, "profile-a", START_TIME, {
        repository_id: repositoryId("repository-a"),
      }),
      session("session-b", "profile-a", SECOND_TIME, {
        repository_id: repositoryId("repository-b"),
      }),
    ];
    mocks.agentProfiles = [profile("profile-a", "Alpha", "claude")];
    mocks.repositoriesByWorkspaceId = {
      "workspace-1": [repository("repository-a", "Frontend"), repository("repository-b", "API")],
    };

    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);

    expect(
      screen.getByRole("button", {
        name: "Active session: Alpha. Repository: Frontend. Tap to switch.",
      }),
    ).toBeTruthy();
    expect(screen.getByTestId(PILL_TESTID).textContent).toContain("Alpha · Frontend");

    fireEvent.click(screen.getByTestId(PILL_TESTID));
    expect(
      within(screen.getByTestId("mobile-session-row-session-a")).getByText("Frontend"),
    ).toBeTruthy();
    expect(
      within(screen.getByTestId("mobile-session-row-session-b")).getByText("API"),
    ).toBeTruthy();

    fireEvent.click(screen.getByTestId("mobile-session-row-session-b"));
    expect(mocks.setActiveSession).toHaveBeenCalledWith(TASK_ID, "session-b");
  });
});

describe("MobileSessionsPicker activity precedence", () => {
  it("renders background-running distinctly — matching desktop, not a done check", () => {
    // A session whose
    // foreground turn is idle while spawned background work runs (RUNNING +
    // `background`) must read as background-running on mobile too — the shared
    // getSessionStateIcon spinner — distinct from generating and never a done
    // check. Tabler renders the icon shape into the svg class
    // (`tabler-icon-<name>`), so asserting the class proves the distinction is
    // carried by SHAPE (survives a grayscale scan), not hue alone.
    mocks.activeSessionId = SESSION_BG;
    mocks.sessions = [
      session(SESSION_BG, "profile-a", START_TIME, {
        state: "WAITING_FOR_INPUT",
        foreground_activity: "background",
      }),
      session("session-gen", "profile-b", SECOND_TIME, {
        state: "RUNNING",
        foreground_activity: "generating",
      }),
      session("session-done", "profile-a", "2026-01-01T00:02:00Z", {
        state: "COMPLETED",
      }),
    ];
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_BG} fullWidth />);
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

  it("shows pending clarification instead of background-running", () => {
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

    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_BG} fullWidth />);
    fireEvent.click(screen.getByTestId(PILL_TESTID));

    const state = screen.getByTestId("mobile-session-state-session-bg");
    expect(state.textContent).toMatch(/waiting for input/i);
    expect(state.querySelector("svg")?.getAttribute("class")).toContain(
      "tabler-icon-message-question",
    );
    mocks.messagesBySession = {};
  });

  it("shows pending permission ahead of clarification and generating", () => {
    mocks.activeSessionId = SESSION_A;
    mocks.sessions = [
      session(SESSION_A, "profile-a", START_TIME, {
        state: "RUNNING",
        foreground_activity: "generating",
      }),
    ];
    mocks.messagesBySession = {
      [SESSION_A]: [
        { type: "clarification_request", metadata: { status: "pending" } },
        { type: "permission_request", metadata: { status: "pending" } },
      ],
    };

    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);
    fireEvent.click(screen.getByTestId(PILL_TESTID));

    const state = screen.getByTestId("mobile-session-state-session-a");
    expect(state.textContent).toMatch(/permission requested/i);
    expect(state.querySelector("svg")?.getAttribute("class")).toContain(
      "tabler-icon-shield-question",
    );
    mocks.messagesBySession = {};
  });
});

describe("MobileSessionsPicker pending lifecycle", () => {
  it("does not let stale pending input mask starting or terminal labels", () => {
    mocks.activeSessionId = SESSION_A;
    mocks.sessions = [
      session(SESSION_A, "profile-a", START_TIME, {
        state: "STARTING",
        foreground_activity: "background",
      }),
      session("session-done", "profile-b", SECOND_TIME, {
        state: "COMPLETED",
        foreground_activity: "generating",
      }),
    ];
    mocks.messagesBySession = {
      [SESSION_A]: [{ type: "permission_request", metadata: { status: "pending" } }],
      "session-done": [{ type: "clarification_request", metadata: { status: "pending" } }],
    };

    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);
    fireEvent.click(screen.getByTestId(PILL_TESTID));

    expect(screen.getByTestId("mobile-session-state-session-a").textContent).toMatch(/starting/i);
    expect(screen.getByTestId("mobile-session-state-session-done").textContent).toMatch(
      /completed/i,
    );
    mocks.messagesBySession = {};
  });

  it("carries the waiting-for-input variants", () => {
    // A pending clarification and a pending permission each read distinctly on
    // the mobile session row — the question / shield glyphs — never a done check
    // or a running dot, matching the sidebar and desktop menus.
    mocks.activeSessionId = "session-clar";
    mocks.sessions = [
      session("session-clar", "profile-a", START_TIME, {
        state: "WAITING_FOR_INPUT",
      }),
      session("session-perm", "profile-b", SECOND_TIME, {
        state: "WAITING_FOR_INPUT",
      }),
    ];
    mocks.messagesBySession = {
      "session-clar": [{ type: "clarification_request", metadata: { status: "pending" } }],
      "session-perm": [{ type: "permission_request", metadata: { status: "pending" } }],
    };
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId="session-clar" fullWidth />);
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

describe("MobileSessionsPicker session delete confirmation", () => {
  it("morphs the target row into local touch-sized confirmation actions", () => {
    mocks.sessions = [session(SESSION_A, "profile-a", START_TIME, { state: "COMPLETED" })];
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);

    // Open the picker sheet, then the session's actions menu.
    fireEvent.click(screen.getByTestId(PILL_TESTID));
    const dotsButton = screen.getByRole("button", { name: SESSION_ACTIONS_LABEL });
    fireEvent.pointerDown(dotsButton);
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    const confirmation = screen.getByRole("group", { name: /delete session/i });
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(confirmation.textContent).toContain("permanently delete the conversation history");
    expect(confirmation.textContent).toContain("task workspace and its files are kept");
    expect(confirmation.textContent).toContain("only session for this task");
    const confirm = within(confirmation).getByTestId("mobile-session-delete-confirm");
    expect(confirm.className).toContain("h-11");
    expect(confirm.className).toContain("min-w-11");
  });

  it("cancels locally without deleting the session", () => {
    mocks.sessions = [session(SESSION_A, "profile-a", START_TIME, { state: "COMPLETED" })];
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);

    fireEvent.click(screen.getByTestId(PILL_TESTID));
    fireEvent.pointerDown(screen.getByRole("button", { name: SESSION_ACTIONS_LABEL }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(mocks.removeSession).not.toHaveBeenCalled();
    expect(screen.getByTestId(`mobile-session-row-${SESSION_A}`)).toBeTruthy();
    expect(screen.queryByRole("group", { name: /delete session/i })).toBeNull();
  });

  it("resets pending confirmation when the picker closes externally", async () => {
    mocks.sessions = [session(SESSION_A, "profile-a", START_TIME, { state: "COMPLETED" })];
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);

    fireEvent.click(screen.getByTestId(PILL_TESTID));
    fireEvent.pointerDown(screen.getByRole("button", { name: SESSION_ACTIONS_LABEL }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    expect(screen.getByRole("group", { name: /delete session/i })).toBeTruthy();
    const picker = screen.getByRole("dialog", { name: "Sessions" });
    expect(picker.getAttribute("data-state")).toBe("open");

    fireEvent.keyDown(document, { key: "Escape" });

    await vi.waitFor(() => {
      expect(picker.getAttribute("data-state")).toBe("closed");
    });
    expect(mocks.removeSession).not.toHaveBeenCalled();
    expect(screen.queryByRole("group", { name: /delete session/i })).toBeNull();

    fireEvent.click(screen.getByTestId(PILL_TESTID));
    expect(screen.getByRole("dialog", { name: "Sessions" }).getAttribute("data-state")).toBe(
      "open",
    );
    expect(screen.queryByRole("group", { name: /delete session/i })).toBeNull();
  });

  it("dispatches deletion once after local confirmation", async () => {
    mocks.sessions = [session(SESSION_A, "profile-a", START_TIME, { state: "COMPLETED" })];
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);

    fireEvent.click(screen.getByTestId(PILL_TESTID));
    fireEvent.pointerDown(screen.getByRole("button", { name: SESSION_ACTIONS_LABEL }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    fireEvent.click(screen.getByTestId("mobile-session-delete-confirm"));

    await vi.waitFor(() => expect(mocks.removeSession).toHaveBeenCalledTimes(1));
  });
});
