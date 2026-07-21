import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IssueWatchTable } from "./issue-watch-table";
import type { IssueWatch } from "@/lib/types/gitlab";

const watch: IssueWatch = {
  id: "issue-1",
  workspace_id: "ws-1",
  workflow_id: "workflow",
  workflow_step_id: "step",
  projects: [],
  agent_profile_id: "",
  executor_profile_id: "",
  prompt: "fix",
  labels: ["bug"],
  custom_query: "state=opened",
  enabled: false,
  poll_interval_seconds: 600,
  cleanup_policy: "never",
  created_at: "2026-01-01",
  updated_at: "2026-01-01",
};

afterEach(cleanup);

describe("IssueWatchTable", () => {
  it("makes the disabled Check now reason keyboard discoverable", async () => {
    render(
      <TooltipProvider>
        <IssueWatchTable
          watches={[{ ...watch, enabled: true }]}
          dirtyIds={new Set([watch.id])}
          authoritativeEnabledById={new Map([[watch.id, false]])}
          onEdit={vi.fn()}
          onDelete={vi.fn()}
          onTrigger={vi.fn()}
          onReset={vi.fn()}
          onToggleEnabled={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getAllByText("All projects").length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: "Pause watch" }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: "Delete watch" }).length).toBeGreaterThan(0);
    for (const button of screen.getAllByRole("button", { name: "Check now" })) {
      expect((button as HTMLButtonElement).disabled).toBe(true);
      expect(button.getAttribute("aria-description")).toMatch(/save changes/i);
    }
    const reasonTriggers = screen.getAllByLabelText("Save changes before checking now");
    for (const trigger of reasonTriggers) {
      expect(trigger.tagName).toBe("SPAN");
      expect(trigger.getAttribute("tabindex")).toBe("0");
    }
    expect(reasonTriggers.some((trigger) => trigger.className.includes("h-11"))).toBe(true);
    fireEvent.focus(reasonTriggers[0]);
    expect((await screen.findByRole("tooltip")).textContent).toContain(
      "Save changes before checking now",
    );
  });
});
