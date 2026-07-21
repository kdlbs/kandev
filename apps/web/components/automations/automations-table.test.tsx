import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Automation } from "@/lib/types/automation";
import { AutomationsTable } from "./automations-table";

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
}));

const automation = {
  id: "automation-1",
  name: "Daily check",
  enabled: false,
  execution_mode: "task",
  triggers: [],
  last_triggered_at: null,
} as unknown as Automation;

afterEach(cleanup);

describe("AutomationsTable", () => {
  it("marks a changed toggle, its row, and the table container as dirty", () => {
    render(
      <TooltipProvider>
        <AutomationsTable
          automations={[automation]}
          dirtyIds={new Set([automation.id])}
          workspaceId="workspace-1"
          onToggleEnabled={vi.fn()}
          onTrigger={vi.fn()}
          onDelete={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.getByTestId("automations-table").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(
      screen.getByTestId("automation-row-automation-1").getAttribute("data-settings-dirty"),
    ).toBe("true");
    expect(
      screen.getByTestId("automation-enabled-automation-1").getAttribute("data-settings-dirty"),
    ).toBe("true");
  });
});
