import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { TaskSession } from "@/lib/types/http";
import { agentProfileId, sessionId, taskId } from "@/lib/types/ids";
import type { ResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const responsiveMocks = vi.hoisted(() => ({
  useResponsiveBreakpoint: vi.fn(),
}));
const storeMocks = vi.hoisted(() => ({
  useAppStore: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => responsiveMocks);
vi.mock("@/components/state-provider", () => storeMocks);

import { ThreadSessionSwitcher } from "./thread-session-switcher";

const SESSION_A = "session-a";
const SESSION_B = "session-b";
const PROFILE_FAST = "profile-fast";
const AGENT_PROFILES: AgentProfileOption[] = [
  {
    id: PROFILE_FAST,
    label: "Mock • Mock Fast",
    agent_id: "mock",
    agent_name: "mock",
    cli_passthrough: false,
  },
  {
    id: "profile-smart",
    label: "Mock • Mock Smart",
    agent_id: "mock",
    agent_name: "mock",
    cli_passthrough: false,
  },
];

function session(id: string, name: string, overrides: Partial<TaskSession> = {}): TaskSession {
  return {
    id: sessionId(id),
    task_id: taskId("task-1"),
    name,
    state: "RUNNING",
    started_at: "2026-08-27T10:00:00Z",
    updated_at: "2026-08-27T12:00:00Z",
    ...overrides,
  };
}

function desktop(): ResponsiveBreakpoint {
  return { isMobile: false } as ResponsiveBreakpoint;
}

function mobile(): ResponsiveBreakpoint {
  return { isMobile: true } as ResponsiveBreakpoint;
}

afterEach(cleanup);

beforeEach(() => {
  responsiveMocks.useResponsiveBreakpoint.mockReturnValue(desktop());
  storeMocks.useAppStore.mockImplementation(
    (selector: (state: { agentProfiles: { items: AgentProfileOption[] } }) => unknown) =>
      selector({ agentProfiles: { items: AGENT_PROFILES } }),
  );
});

describe("ThreadSessionSwitcher — desktop identity", () => {
  it("shows the effective agent profile name instead of a session id", () => {
    render(
      <ThreadSessionSwitcher
        sessions={[
          session(SESSION_A, "Old session title", {
            state: "IDLE",
            agent_profile_id: agentProfileId(PROFILE_FAST),
          }),
          session(SESSION_B, "Builder", { state: "IDLE" }),
        ]}
        selectedSessionId={SESSION_A}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByRole("tab", { name: /Mock Fast/ })).not.toBeNull();
    expect(screen.queryByRole("tab", { name: /Old session title/ })).toBeNull();
    expect(screen.queryByRole("tab", { name: new RegExp(SESSION_A) })).toBeNull();
  });

  it("shows the grid spinner while the agent is running", () => {
    render(
      <ThreadSessionSwitcher
        sessions={[
          session(SESSION_A, "", {
            state: "RUNNING",
            agent_profile_id: agentProfileId(PROFILE_FAST),
          }),
          session(SESSION_B, "Builder", { state: "IDLE" }),
        ]}
        selectedSessionId={SESSION_A}
        onSelect={vi.fn()}
      />,
    );

    const runningTab = screen.getByRole("tab", { name: /Mock Fast/ });
    expect(within(runningTab).getByRole("status", { name: "Loading" })).not.toBeNull();
  });

  it("shows the effective agent logo when the session is settled", () => {
    render(
      <ThreadSessionSwitcher
        sessions={[
          session(SESSION_A, "", {
            state: "IDLE",
            agent_profile_id: agentProfileId(PROFILE_FAST),
          }),
          session(SESSION_B, "Builder", { state: "IDLE" }),
        ]}
        selectedSessionId={SESSION_A}
        onSelect={vi.fn()}
      />,
    );

    const settledTab = screen.getByRole("tab", { name: /Mock Fast/ });
    expect(within(settledTab).getByTestId(`thread-session-agent-icon-${SESSION_A}`)).not.toBeNull();
    expect(within(settledTab).queryByLabelText("Turn finished")).toBeNull();
  });

  it("shows explicit pending attention on the session item", () => {
    render(
      <ThreadSessionSwitcher
        sessions={[
          session(SESSION_A, "Question", {
            state: "WAITING_FOR_INPUT",
            pending_action: "clarification",
          }),
          session(SESSION_B, "Builder", { state: "IDLE" }),
        ]}
        selectedSessionId={SESSION_B}
        onSelect={vi.fn()}
      />,
    );

    const questionTab = screen.getByRole("tab", { name: /Question/ });
    expect(within(questionTab).getByLabelText("Question from agent")).not.toBeNull();
    expect(within(questionTab).queryByTestId(`thread-session-agent-icon-${SESSION_A}`)).toBeNull();
  });
});

describe("ThreadSessionSwitcher — desktop interaction", () => {
  it("renders switch-only tabs in the existing desktop metadata row", () => {
    render(
      <ThreadSessionSwitcher
        sessions={[session(SESSION_A, "Planner"), session(SESSION_B, "Builder")]}
        selectedSessionId={SESSION_A}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByTestId("thread-session-switcher")).not.toBeNull();
    expect(screen.getByRole("tab", { name: /Planner/ })).not.toBeNull();
    expect(screen.getByRole("tab", { name: /Builder/ })).not.toBeNull();
    expect(screen.queryByRole("button", { name: /add|new/i })).toBeNull();
  });

  it("reports a selected desktop session without changing global task state", () => {
    const onSelect = vi.fn();
    render(
      <ThreadSessionSwitcher
        sessions={[session(SESSION_A, "Planner"), session(SESSION_B, "Builder")]}
        selectedSessionId={SESSION_A}
        onSelect={onSelect}
      />,
    );

    fireEvent.mouseDown(screen.getByRole("tab", { name: /Builder/ }), { button: 0 });

    expect(onSelect).toHaveBeenCalledWith(SESSION_B);
  });

  it("keeps long desktop labels inside the constrained metadata flex item", () => {
    render(
      <ThreadSessionSwitcher
        sessions={[
          session(SESSION_A, "Planner with a deliberately long session label"),
          session(SESSION_B, "Builder with another deliberately long session label"),
        ]}
        selectedSessionId={SESSION_A}
        onSelect={vi.fn()}
      />,
    );

    const switcher = screen.getByTestId("thread-session-switcher");
    const tabs = screen.getByTestId("thread-session-tabs");
    const tabList = screen.getByRole("tablist");
    expect(switcher.className).toContain("min-w-0");
    expect(switcher.className).toContain("max-w-[52%]");
    expect(switcher.className).toContain("shrink");
    expect(tabs.className).toContain("w-full");
    expect(tabList.className).toContain("w-full");
    expect(tabList.className).toContain("overflow-x-auto");
  });
});

describe("ThreadSessionSwitcher — phone picker", () => {
  it("uses a phone pill and a 44-pixel bottom-sheet row instead of tabs", () => {
    responsiveMocks.useResponsiveBreakpoint.mockReturnValue(mobile());
    const onSelect = vi.fn();
    render(
      <ThreadSessionSwitcher
        sessions={[session(SESSION_A, "Planner"), session(SESSION_B, "Builder")]}
        selectedSessionId={SESSION_A}
        onSelect={onSelect}
      />,
    );

    expect(screen.queryByRole("tab")).toBeNull();
    fireEvent.click(screen.getByTestId("thread-session-picker-trigger"));

    const row = screen.getByTestId(`thread-session-row-${SESSION_B}`);
    expect(row.className).toContain("min-h-11");
    fireEvent.click(row);

    expect(onSelect).toHaveBeenCalledWith(SESSION_B);
  });

  it("uses the same running spinner and agent logo in the phone picker", () => {
    responsiveMocks.useResponsiveBreakpoint.mockReturnValue(mobile());
    render(
      <ThreadSessionSwitcher
        sessions={[
          session(SESSION_A, "", {
            state: "RUNNING",
            agent_profile_id: agentProfileId(PROFILE_FAST),
          }),
          session(SESSION_B, "", {
            state: "IDLE",
            agent_profile_id: agentProfileId("profile-smart"),
          }),
        ]}
        selectedSessionId={SESSION_A}
        onSelect={vi.fn()}
      />,
    );

    fireEvent.click(screen.getByTestId("thread-session-picker-trigger"));

    const runningRow = screen.getByTestId(`thread-session-row-${SESSION_A}`);
    const settledRow = screen.getByTestId(`thread-session-row-${SESSION_B}`);
    expect(within(runningRow).getByRole("status", { name: "Loading" })).not.toBeNull();
    expect(within(settledRow).getByTestId(`thread-session-agent-icon-${SESSION_B}`)).not.toBeNull();
  });
});
