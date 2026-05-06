import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup, within } from "@testing-library/react";

vi.mock("next/link", () => ({
  default: ({ children, href, ...rest }: { children: React.ReactNode; href: string }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import { AgentCard } from "./agent-card";
import type { AgentSummary, SessionSummary } from "@/lib/api/domains/office-api";

afterEach(() => cleanup());

const TASK_PILL_TESTID = "agent-card-task-pill";
const LIVE_DOT_TESTID = "agent-card-live-dot";
const SUBTITLE_TESTID = "agent-card-subtitle";

function makeSession(overrides: Partial<SessionSummary> = {}): SessionSummary {
  return {
    session_id: "sess-1",
    task_id: "task-1",
    task_identifier: "KAN-3",
    task_title: "present yourself",
    state: "RUNNING",
    started_at: new Date(Date.now() - 60_000).toISOString(),
    completed_at: null,
    duration_seconds: 60,
    command_count: 0,
    ...overrides,
  };
}

function makeSummary(overrides: Partial<AgentSummary> = {}): AgentSummary {
  return {
    agent_id: "ceo",
    agent_name: "CEO",
    agent_role: "ceo",
    status: "never_run",
    live_session: null,
    last_session: null,
    recent_sessions: [],
    ...overrides,
  };
}

describe("AgentCard", () => {
  it("renders never_run state with no live dot, no pill, no rows", () => {
    render(<AgentCard summary={makeSummary()} />);
    expect(screen.getByTestId("agent-card-ceo")).toBeTruthy();
    expect(screen.getByTestId(SUBTITLE_TESTID).textContent).toBe("Never run");
    expect(screen.queryByTestId(LIVE_DOT_TESTID)).toBeNull();
    expect(screen.queryByTestId(TASK_PILL_TESTID)).toBeNull();
  });

  it("renders live state with pulsing dot, 'Live now' subtitle, and task pill", () => {
    const live = makeSession({ state: "RUNNING" });
    render(
      <AgentCard
        summary={makeSummary({
          status: "live",
          live_session: live,
          last_session: live,
          recent_sessions: [live],
        })}
      />,
    );
    expect(screen.getByTestId(LIVE_DOT_TESTID)).toBeTruthy();
    expect(screen.getByTestId(SUBTITLE_TESTID).textContent).toBe("Live now");
    const pill = screen.getByTestId(TASK_PILL_TESTID);
    expect(pill).toBeTruthy();
    expect(within(pill).getByText("KAN-3")).toBeTruthy();
    expect(within(pill).getByText("present yourself")).toBeTruthy();
  });

  it("renders finished state without live dot, with task pill, and a Finished subtitle", () => {
    const last = makeSession({
      state: "COMPLETED",
      completed_at: new Date(Date.now() - 5 * 60_000).toISOString(),
      command_count: 3,
    });
    render(
      <AgentCard
        summary={makeSummary({
          status: "finished",
          last_session: last,
          recent_sessions: [last],
        })}
      />,
    );
    expect(screen.queryByTestId(LIVE_DOT_TESTID)).toBeNull();
    expect(screen.getByTestId(SUBTITLE_TESTID).textContent).toMatch(/^Finished /);
    expect(screen.getByTestId(TASK_PILL_TESTID)).toBeTruthy();
  });

  it("caps recent sessions list at 5 even if backend returned more", () => {
    const recent = Array.from({ length: 8 }, (_, i) =>
      makeSession({
        session_id: `s-${i}`,
        state: "COMPLETED",
        completed_at: new Date(Date.now() - i * 60_000).toISOString(),
      }),
    );
    render(
      <AgentCard
        summary={makeSummary({
          status: "finished",
          last_session: recent[0],
          recent_sessions: recent,
        })}
      />,
    );
    // Each row contains "worked for"; live rows say "working for".
    const rows = screen.getAllByText(/worked for/);
    expect(rows.length).toBeLessThanOrEqual(5);
    expect(rows.length).toBe(5);
  });

  it("does not render a task pill when there are no sessions", () => {
    render(<AgentCard summary={makeSummary({ status: "never_run" })} />);
    expect(screen.queryByTestId(TASK_PILL_TESTID)).toBeNull();
  });
});
