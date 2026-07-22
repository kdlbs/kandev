import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewWatchTable } from "./review-watch-table";
import type { ReviewWatch } from "@/lib/types/gitlab";

const watch: ReviewWatch = {
  id: "review-1",
  workspace_id: "ws-1",
  workflow_id: "workflow",
  workflow_step_id: "step",
  projects: [{ path: "group/api" }],
  agent_profile_id: "",
  executor_profile_id: "",
  prompt: "review",
  review_scope: "user",
  custom_query: "state=opened",
  enabled: true,
  poll_interval_seconds: 300,
  cleanup_policy: "auto",
  last_error: "Workflow step was deleted",
  created_at: "2026-01-01",
  updated_at: "2026-01-01",
};

afterEach(cleanup);

describe("ReviewWatchTable", () => {
  it("keeps Check now disabled while an active watch has a paused draft", () => {
    render(
      <TooltipProvider>
        <ReviewWatchTable
          watches={[{ ...watch, enabled: false }]}
          dirtyIds={new Set([watch.id])}
          authoritativeEnabledById={new Map([[watch.id, true]])}
          onEdit={vi.fn()}
          onDelete={vi.fn()}
          onTrigger={vi.fn()}
          onReset={vi.fn()}
          onToggleEnabled={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getAllByText("Workflow step was deleted").length).toBeGreaterThan(0);
    for (const button of screen.getAllByRole("button", { name: "Check now" })) {
      expect((button as HTMLButtonElement).disabled).toBe(true);
      expect(button.getAttribute("aria-description")).toMatch(/save changes/i);
    }
    expect(screen.getAllByRole("button", { name: "Reset watch" }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: "Edit watch" }).length).toBeGreaterThan(0);
  });
});
