import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { BudgetPolicy } from "@/lib/state/slices/office/types";
import { BudgetPolicyCard } from "./budget-policy-card";

const policy: BudgetPolicy = {
  id: "policy-1",
  workspaceId: "workspace-1",
  scopeType: "workspace",
  scopeId: "workspace-1",
  limitSubcents: 100000,
  period: "total",
  alertThresholdPct: 80,
  actionOnExceed: "notify_only",
  createdAt: "2026-10-14T10:18:52Z",
  updatedAt: "2026-10-14T10:18:52Z",
};

afterEach(cleanup);

// @covers AC-OFFICE-COSTS-001.9
describe("budget observation", () => {
  it.each([{}, { spentSubcents: undefined }])(
    "shows unavailable usage for absent observation %j",
    (observation) => {
      const { container } = render(<BudgetPolicyCard policy={policy} {...observation} />);

      expect(screen.queryByText("Healthy")).toBeNull();
      expect(screen.getByText("Unknown")).toBeTruthy();
      expect(screen.getByText("Unavailable")).toBeTruthy();
      expect(screen.getByText("Remaining: Unavailable")).toBeTruthy();
      expect(screen.queryByText(/\$0\.00|\(0%\)|Remaining: \$10\.00/)).toBeNull();
      expect(container.querySelector(".bg-green-500, .bg-yellow-500, .bg-red-500")).toBeNull();
      expect(screen.getByText("Unknown").className).not.toMatch(/bg-(green|yellow|red)-/);
      expect(container.querySelector(".h-2.bg-muted")).toBeTruthy();
    },
  );

  it.each([
    { spent: 0, observed: "$0.00 (0%)", remaining: "$10.00", status: "Healthy", color: "green" },
    {
      spent: 25000,
      observed: "$2.50 (25%)",
      remaining: "$7.50",
      status: "Healthy",
      color: "green",
    },
    {
      spent: 85000,
      observed: "$8.50 (85%)",
      remaining: "$1.50",
      status: "Warning",
      color: "yellow",
    },
    {
      spent: 120000,
      observed: "$12.00 (100%)",
      remaining: "$0.00",
      status: "Exceeded",
      color: "red",
    },
  ])("keeps measured usage at $spent subcents", ({ spent, observed, remaining, status, color }) => {
    const { container } = render(<BudgetPolicyCard policy={policy} spentSubcents={spent} />);

    expect(screen.getByText(observed)).toBeTruthy();
    expect(screen.getByText(`Remaining: ${remaining}`)).toBeTruthy();
    expect(screen.getByText(status)).toBeTruthy();
    expect(screen.queryByText("Unknown")).toBeNull();
    expect(screen.queryByText("Unavailable")).toBeNull();
    expect(container.querySelector(`.bg-${color}-500`)).toBeTruthy();
  });

  it("keeps configuration and the existing delete callback when usage is unknown", () => {
    const onDelete = vi.fn();
    render(
      <TooltipProvider>
        <BudgetPolicyCard policy={policy} onDelete={onDelete} />
      </TooltipProvider>,
    );

    expect(screen.getByText("workspace: workspace-1")).toBeTruthy();
    expect(screen.getByText("Budget: $10.00")).toBeTruthy();
    expect(screen.getByText("Period: total")).toBeTruthy();
    expect(screen.getByText("Alert: 80%")).toBeTruthy();
    expect(screen.getByText("Action: notify only")).toBeTruthy();
    fireEvent.click(screen.getByRole("button"));
    expect(onDelete).toHaveBeenCalledExactlyOnceWith("policy-1");
  });
});
