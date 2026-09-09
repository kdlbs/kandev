import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { QueuePanelHeader, type QueuePanelHeaderProps } from "./queued-ghost-panel-header";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

const props: QueuePanelHeaderProps = {
  count: 1,
  max: 10,
  isFull: false,
  autoRun: false,
  isLoading: false,
  cancellationPending: false,
  pinned: false,
  onClear: vi.fn(),
  onAutoRunChange: vi.fn(),
  onTogglePin: vi.fn(),
  onClose: vi.fn(),
};

describe("QueuePanelHeader", () => {
  it("connects each rendered Auto-run switch to unique label and help IDs", () => {
    const { container } = render(
      <>
        <QueuePanelHeader {...props} />
        <QueuePanelHeader {...props} />
      </>,
    );

    const switches = screen.getAllByTestId("queue-auto-run");
    const labels = Array.from(container.querySelectorAll("label"));
    expect(new Set(switches.map((control) => control.id)).size).toBe(2);
    expect(new Set(switches.map((control) => control.getAttribute("aria-describedby"))).size).toBe(
      2,
    );
    switches.forEach((control, index) => {
      expect(labels[index].htmlFor).toBe(control.id);
      const helpId = control.getAttribute("aria-describedby");
      expect(helpId).toBeTruthy();
      expect(container.querySelectorAll(`[id="${helpId}"]`)).toHaveLength(1);
    });
  });
});
