import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MobileSessionsPicker } from "./mobile-sessions-section";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  activeSessionId: "session-a" as string | null,
  sessions: [] as TaskSession[],
  agentProfiles: [] as AgentProfileOption[],
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

function session(id: string, profileId: string, startedAt: string): TaskSession {
  return {
    id,
    task_id: "task-1",
    agent_profile_id: profileId,
    state: "WAITING_FOR_INPUT",
    started_at: startedAt,
    updated_at: startedAt,
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

describe("MobileSessionsPicker", () => {
  afterEach(cleanup);

  beforeEach(() => {
    mocks.activeSessionId = "session-a";
    mocks.sessions = [
      session("session-a", "profile-a", "2026-01-01T00:00:00Z"),
      session("session-b", "profile-b", "2026-01-01T00:01:00Z"),
    ];
    mocks.agentProfiles = [
      profile("profile-a", "Alpha", "claude"),
      profile("profile-b", "Beta", "codex"),
    ];
  });

  it("uses the effective layout session instead of a stale store session", () => {
    render(<MobileSessionsPicker taskId="task-1" sessionId="session-b" fullWidth />);

    expect(
      screen.getByRole("button", { name: "Active session: Beta #2. Tap to switch." }),
    ).toBeTruthy();

    fireEvent.click(screen.getByTestId("mobile-sessions-pill"));
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

    const pill = screen.getByTestId("mobile-sessions-pill");
    expect(within(pill).getByTestId("mobile-session-agent-icon")).toBeTruthy();
    expect(within(pill).getByTestId("agent-logo-codex")).toBeTruthy();
  });
});
