import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { QueuePanelHeader, type QueuePanelHeaderProps } from "./queued-ghost-panel-header";
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
afterEach(cleanup);

const props: QueuePanelHeaderProps = {
  count: 1,
  max: 10,
  isFull: false,
  autoRun: false,
  autoMerge: true,
  autoMergeAvailable: true,
  autoSortMerge: true,
  autoSortMergeAvailable: true,
  isLoading: false,
  cancellationPending: false,
  pinned: false,
  onClear: vi.fn(),
  onAutoRunChange: vi.fn(),
  onAutoMergeChange: vi.fn(),
  onAutoSortMergeChange: vi.fn(),
  onTogglePin: vi.fn(),
  onClose: vi.fn(),
};

describe("QueuePanelHeader", () => {
  it("renders compact Auto-run and Auto-merge pills with unique accessible labels", () => {
    const { container } = render(
      <TooltipProvider>
        <QueuePanelHeader {...props} />
        <QueuePanelHeader {...props} />
      </TooltipProvider>,
    );

    const autoRunSwitches = screen.getAllByTestId("queue-auto-run");
    const autoMergeSwitches = screen.getAllByTestId("queue-auto-merge");
    expect(autoRunSwitches).toHaveLength(2);
    expect(autoMergeSwitches).toHaveLength(2);
    for (const controls of [autoRunSwitches, autoMergeSwitches]) {
      expect(new Set(controls.map((control) => control.id)).size).toBe(2);
      controls.forEach((control) => {
        const label = container.querySelector(`label[for="${control.id}"]`);
        expect(label).not.toBeNull();
        expect(control.className).toContain("[@media(pointer:coarse)]:after:-inset-y-3.5");
      });
    }
    const descriptionId = autoRunSwitches[0].getAttribute("aria-describedby");
    expect(descriptionId).not.toBeNull();
    expect(document.getElementById(descriptionId ?? "")).not.toBeNull();
  });

  it("disables both policies through the same conflicting-operation state", () => {
    render(
      <TooltipProvider>
        <QueuePanelHeader {...props} isLoading />
      </TooltipProvider>,
    );
    expect((screen.getByTestId("queue-auto-run") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("queue-auto-merge") as HTMLButtonElement).disabled).toBe(true);
  });

  it("disables Clear All while another queue mutation is in flight", () => {
    render(
      <TooltipProvider>
        <QueuePanelHeader {...props} isLoading />
      </TooltipProvider>,
    );

    expect((screen.getByTestId("queue-clear-all") as HTMLButtonElement).disabled).toBe(true);
  });

  it("keeps the unavailable Auto-merge tooltip keyboard reachable", () => {
    const { container } = render(
      <TooltipProvider>
        <QueuePanelHeader {...props} autoMergeAvailable={false} />
      </TooltipProvider>,
    );

    const autoMergeSwitch = screen.getByTestId("queue-auto-merge") as HTMLButtonElement;
    expect(autoMergeSwitch.disabled).toBe(true);
    const label = container.querySelector(`label[for="${autoMergeSwitch.id}"]`);
    expect(label?.className).toContain("opacity-50");
    expect(label?.className).toContain("pointer-events-none");
    expect(label?.parentElement?.tagName).toBe("SPAN");
    expect(label?.parentElement?.getAttribute("tabindex")).toBe("0");
  });
});

describe("Auto-sort-merge dependency", () => {
  it.each([
    {
      autoMerge: false,
      autoMergeAvailable: true,
      autoSortMergeAvailable: true,
      checked: false,
      disabled: true,
    },
    {
      autoMerge: true,
      autoMergeAvailable: false,
      autoSortMergeAvailable: true,
      checked: true,
      disabled: true,
    },
    {
      autoMerge: true,
      autoMergeAvailable: true,
      autoSortMergeAvailable: false,
      checked: true,
      disabled: true,
    },
    {
      autoMerge: true,
      autoMergeAvailable: true,
      autoSortMergeAvailable: true,
      checked: true,
      disabled: false,
    },
  ])("renders the effective dependency state %j", ({ checked, disabled, ...policy }) => {
    render(
      <TooltipProvider>
        <QueuePanelHeader {...props} {...policy} />
      </TooltipProvider>,
    );
    const control = screen.getByTestId("queue-auto-sort-merge");
    expect(control.getAttribute("aria-checked")).toBe(String(checked));
    expect(control).toHaveProperty("disabled", disabled);
    const description = document.getElementById(control.getAttribute("aria-describedby") ?? "");
    expect(description).not.toBeNull();
  });
});
