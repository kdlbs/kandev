import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ReviewWatchTable } from "./review-watch-table";
import { formatDateTime } from "@/lib/i18n/formats";
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

  // The last-checked cell used to call a bare `toLocaleString()`, which follows
  // the BROWSER locale and so could disagree with the rest of the page after a
  // language switch. Asserting against `formatDateTime` — which reads the active
  // i18next language — pins it to the application locale: a revert to
  // `toLocaleString()` produces a different string ("1/2/2026, 10:00:00 AM" vs
  // "Jan 2, 2026, 10:00 AM") and fails here.
  it.each([
    { name: "a polled watch", lastPolledAt: "2026-01-02T10:00:00Z" },
    { name: "a watch polled at a different instant", lastPolledAt: "2026-06-30T23:15:00Z" },
  ])("renders $name's last-checked time in the application locale", ({ lastPolledAt }) => {
    render(
      <TooltipProvider>
        <ReviewWatchTable
          watches={[{ ...watch, last_polled_at: lastPolledAt }]}
          dirtyIds={new Set()}
          authoritativeEnabledById={new Map()}
          onEdit={vi.fn()}
          onDelete={vi.fn()}
          onTrigger={vi.fn()}
          onReset={vi.fn()}
          onToggleEnabled={vi.fn()}
        />
      </TooltipProvider>,
    );

    const expected = formatDateTime(lastPolledAt);
    expect(expected).not.toBe(new Date(lastPolledAt).toLocaleString());
    // Desktop cell plus the mobile card's "Last checked {{value}}" line.
    expect(screen.getAllByText(new RegExp(escapeRegExp(expected))).length).toBeGreaterThan(1);
  });

  it("falls back to the translated Never when a watch has not been polled", () => {
    render(
      <TooltipProvider>
        <ReviewWatchTable
          watches={[{ ...watch, last_polled_at: undefined }]}
          dirtyIds={new Set()}
          authoritativeEnabledById={new Map()}
          onEdit={vi.fn()}
          onDelete={vi.fn()}
          onTrigger={vi.fn()}
          onReset={vi.fn()}
          onToggleEnabled={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getAllByText(/Never/).length).toBeGreaterThan(1);
  });
});

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
