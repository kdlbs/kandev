import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { WorkflowSyncedBadge } from "./workflow-synced-badge";

/**
 * The tooltip interpolates `sourcePath` — a repository path, i.e. data — and
 * falls back to a translated phrase when the workflow carries none. jsdom does
 * not open a Radix tooltip from synthetic hover, so the sentence is asserted
 * through the keyboard-focus path (see apps/web/AGENTS.md).
 */
describe("WorkflowSyncedBadge", () => {
  afterEach(cleanup);

  it("interpolates the source path into the read-only explanation", async () => {
    render(
      <TooltipProvider>
        <WorkflowSyncedBadge sourcePath=".kandev/workflows/review.yml" />
      </TooltipProvider>,
    );
    screen.getByTestId("workflow-synced-badge").focus();
    const tooltip = await screen.findAllByText(
      "Read-only - managed by workflow sync from .kandev/workflows/review.yml. Edit or remove it in the synced repository.",
    );
    expect(tooltip.length).toBeGreaterThan(0);
  });

  it("falls back to a translated phrase when no source path is known", async () => {
    render(
      <TooltipProvider>
        <WorkflowSyncedBadge />
      </TooltipProvider>,
    );
    screen.getByTestId("workflow-synced-badge").focus();
    const tooltip = await screen.findAllByText(
      "Read-only - managed by workflow sync from a configured repository. Edit or remove it in the synced repository.",
    );
    expect(tooltip.length).toBeGreaterThan(0);
  });
});
