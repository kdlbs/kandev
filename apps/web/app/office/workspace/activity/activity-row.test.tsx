import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import { ActivityRow } from "./activity-row";

function activity(overrides: Partial<ActivityEntry> = {}): ActivityEntry {
  return {
    id: "activity-1",
    workspaceId: "ws-1",
    actorType: "agent",
    actorId: "agent-1",
    action: "task_status_changed",
    targetType: "task",
    targetId: "task-1",
    details: { new_status: "in_review", task_identifier: "KAN-1" },
    createdAt: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}

describe("ActivityRow", () => {
  afterEach(() => {
    cleanup();
  });

  it("links runtime activity back to the originating run", () => {
    render(<ActivityRow entry={activity({ runId: "run-1", sessionId: "sess-1" })} />);

    const link = screen.getByRole("link", { name: "Run" });
    expect(link.getAttribute("href")).toBe("/office/agents/agent-1/runs/run-1");
  });

  it("does not show a run link for non-runtime activity", () => {
    render(<ActivityRow entry={activity()} />);

    expect(screen.queryByRole("link", { name: "Run" })).toBeNull();
  });

  it("links scheduler-emitted run activity using details.agent_id", () => {
    render(
      <ActivityRow
        entry={activity({
          actorType: "system" as ActivityEntry["actorType"],
          actorId: "office-scheduler",
          action: "run_processed",
          targetType: "run",
          targetId: "run-1",
          runId: "run-1",
          details: { agent: "doc-bot", agent_id: "agent-7" },
        })}
      />,
    );

    const link = screen.getByRole("link", { name: "Run" });
    expect(link.getAttribute("href")).toBe("/office/agents/agent-7/runs/run-1");
  });
});
