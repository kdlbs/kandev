import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import { TriggersSection } from "./triggers-section";
import type { AutomationTrigger } from "@/lib/types/automation";

function scheduledTrigger(config: Record<string, unknown>): AutomationTrigger {
  return {
    id: "trigger-1",
    automation_id: "automation-1",
    type: "scheduled",
    config,
    enabled: true,
    created_at: "2026-07-30T00:00:00Z",
    updated_at: "2026-07-30T00:00:00Z",
  } as AutomationTrigger;
}

function renderTriggersSection(overrides: Partial<ComponentProps<typeof TriggersSection>> = {}) {
  const props = {
    triggers: [],
    savedTriggers: [],
    automationId: "automation-1",
    workspaceId: "workspace-1",
    triggerTypes: [],
    onAddTrigger: vi.fn(),
    onUpdateTrigger: vi.fn(),
    onToggleTrigger: vi.fn(),
    onDeleteTrigger: vi.fn(),
    ...overrides,
  } satisfies ComponentProps<typeof TriggersSection>;

  render(
    <TooltipProvider>
      <TriggersSection {...props} />
    </TooltipProvider>,
  );
  return props;
}

afterEach(cleanup);

describe("TriggersSection schedule config", () => {
  it("keeps the timezone when only the time changes", () => {
    const trigger = scheduledTrigger({
      cron_expression: "0 9 * * *",
      timezone: "Asia/Singapore",
    });
    const props = renderTriggersSection({ triggers: [trigger], savedTriggers: [trigger] });

    fireEvent.change(screen.getByTestId("schedule-time"), { target: { value: "10:30" } });

    expect(props.onUpdateTrigger).toHaveBeenCalledWith("trigger-1", {
      cron_expression: "30 10 * * *",
      timezone: "Asia/Singapore",
    });
  });

  it("preserves config keys the schedule control does not own", () => {
    // onUpdateTrigger swaps the config wholesale, so anything the schedule
    // control does not emit has to survive by being merged in.
    const trigger = scheduledTrigger({
      cron_expression: "0 9 * * *",
      timezone: "Asia/Singapore",
      some_future_key: "keep me",
    });
    const props = renderTriggersSection({ triggers: [trigger], savedTriggers: [trigger] });

    fireEvent.change(screen.getByTestId("schedule-time"), { target: { value: "10:30" } });

    expect(props.onUpdateTrigger).toHaveBeenCalledWith(
      "trigger-1",
      expect.objectContaining({ some_future_key: "keep me" }),
    );
  });

  it("creates a scheduled trigger when none exists yet", () => {
    const props = renderTriggersSection();

    fireEvent.click(screen.getByTestId("schedule-frequency"));
    fireEvent.click(screen.getByRole("option", { name: "every day" }));

    expect(props.onAddTrigger).toHaveBeenCalledWith(
      "scheduled",
      expect.objectContaining({ cron_expression: "0 9 * * *" }),
    );
  });
});
