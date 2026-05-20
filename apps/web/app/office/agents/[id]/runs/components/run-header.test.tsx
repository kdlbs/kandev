import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { RunHeader } from "./run-header";
import type { RunDetail } from "@/lib/api/domains/office-extended-api";

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => null,
}));

afterEach(() => {
  cleanup();
});

const CLAUDE = "claude-acp";
const CODEX = "codex-acp";

const baseRun: RunDetail = {
  id: "run-deadbeef-1234-5678",
  id_short: "run-dead",
  agent_id: "agent-1",
  reason: "task_assigned",
  status: "finished",
  task_id: "task-1",
  requested_at: "2026-05-01T12:00:00Z",
  claimed_at: "2026-05-01T12:00:01Z",
  finished_at: "2026-05-01T12:00:31Z",
  duration_ms: 30_000,
  costs: {
    input_tokens: 100,
    output_tokens: 200,
    cached_tokens: 50,
    cost_subcents: 4200,
  },
  session: { session_id: "sess-1" },
  invocation: { adapter: "claude_local", model: "claude-sonnet-4-6" },
  runtime: { capabilities: {}, input_snapshot: {}, skills: [] },
  tasks_touched: ["task-1"],
  events: [],
};

function withRouting(extra: NonNullable<RunDetail["routing"]>): RunDetail {
  return { ...baseRun, routing: extra };
}

describe("RunHeader", () => {
  it("renders the status badge with the run status", () => {
    render(<RunHeader run={baseRun} />);
    expect(screen.getByTestId("run-status-badge").textContent).toContain("finished");
  });

  it("renders adapter and model with a separator", () => {
    render(<RunHeader run={baseRun} />);
    expect(screen.getByTestId("run-adapter").textContent).toContain(
      "claude_local · claude-sonnet-4-6",
    );
  });

  it("formats the duration in human-readable units", () => {
    render(<RunHeader run={baseRun} />);
    expect(screen.getByTestId("run-duration").textContent).toContain("30s");
  });

  it("formats the cost in dollars", () => {
    render(<RunHeader run={baseRun} />);
    expect(screen.getByTestId("run-cost").textContent).toContain("$0.42");
  });

  it("renders token totals", () => {
    render(<RunHeader run={baseRun} />);
    const text = screen.getByTestId("run-tokens").textContent ?? "";
    expect(text).toContain("In:");
    expect(text).toContain("100");
    expect(text).toContain("Out:");
    expect(text).toContain("200");
  });

  it("shows resume + start fresh actions on FAILED runs with a session id", () => {
    const failed: RunDetail = {
      ...baseRun,
      status: "failed",
      error_message: "boom",
    };
    render(<RunHeader run={failed} />);
    expect(screen.getByTestId("run-resume-button")).toBeTruthy();
    expect(screen.getByTestId("run-fresh-start-button")).toBeTruthy();
    expect(screen.getByTestId("run-error-message").textContent).toContain("boom");
  });

  it("shows the cancel action on RUNNING (claimed) runs", () => {
    const running: RunDetail = { ...baseRun, status: "claimed", finished_at: undefined };
    render(<RunHeader run={running} />);
    expect(screen.getByTestId("run-cancel-button")).toBeTruthy();
  });

  it("hides recovery buttons on finished runs", () => {
    render(<RunHeader run={baseRun} />);
    expect(screen.queryByTestId("run-resume-button")).toBeNull();
    expect(screen.queryByTestId("run-cancel-button")).toBeNull();
  });
});

describe("RunHeader routing strip", () => {
  it("omits the routing strip on legacy concrete-profile runs", () => {
    render(<RunHeader run={baseRun} />);
    expect(screen.queryByTestId("run-routing-strip")).toBeNull();
  });

  it("shows resolved only when routed run launched on primary", () => {
    const routed = withRouting({
      logical_provider_order: [CLAUDE, CODEX],
      requested_tier: "balanced",
      resolved_provider_id: CLAUDE,
      resolved_model: "sonnet",
      attempts: [],
    });
    render(<RunHeader run={routed} />);
    expect(screen.getByTestId("run-routing-resolved").textContent).toContain(CLAUDE);
    expect(screen.queryByTestId("run-routing-intended")).toBeNull();
  });

  it("shows intended when routed run fell back to a different provider", () => {
    const routed = withRouting({
      logical_provider_order: [CLAUDE, CODEX],
      requested_tier: "balanced",
      resolved_provider_id: CODEX,
      resolved_model: "gpt-5",
      attempts: [],
    });
    render(<RunHeader run={routed} />);
    expect(screen.getByTestId("run-routing-resolved").textContent).toContain(CODEX);
    expect(screen.getByTestId("run-routing-intended").textContent).toContain(CLAUDE);
  });

  it("renders the waiting-for-capacity badge on parked runs", () => {
    const parked: RunDetail = {
      ...withRouting({
        logical_provider_order: [CLAUDE],
        blocked_status: "waiting_for_provider_capacity",
        earliest_retry_at: "2026-05-01T13:00:00Z",
        attempts: [],
      }),
      status: "queued",
    };
    render(<RunHeader run={parked} />);
    expect(screen.getByTestId("run-routing-waiting")).toBeTruthy();
  });

  it("renders the blocked-action-required badge on blocked runs", () => {
    const blocked: RunDetail = {
      ...withRouting({
        logical_provider_order: [CLAUDE],
        blocked_status: "blocked_provider_action_required",
        attempts: [],
      }),
      status: "queued",
    };
    render(<RunHeader run={blocked} />);
    expect(screen.getByTestId("run-routing-blocked")).toBeTruthy();
  });
});
