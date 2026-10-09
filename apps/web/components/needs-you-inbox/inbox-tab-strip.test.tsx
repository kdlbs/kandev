import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TabsContent } from "@kandev/ui/tabs";
import type { ReactNode } from "react";
import { InboxTabStrip } from "./inbox-tab-strip";
import type { InboxTab } from "@/lib/failed-inbox/inbox-tab";

const FAILED_BADGE_TESTID = "inbox-tab-failed-badge";
const HISTORY_BADGE_TESTID = "inbox-tab-history-badge";

function renderStrip(
  overrides: Partial<{
    selectedTab: InboxTab;
    onSelectTab: (tab: InboxTab) => void;
    needsYouCount: number;
    needsYouHasMore: boolean;
    failedCount: number | undefined;
    failedTruncated: boolean;
    historyCount: number;
    children?: ReactNode;
  }> = {},
) {
  return render(
    <InboxTabStrip
      selectedTab="needs-you"
      onSelectTab={vi.fn()}
      needsYouCount={0}
      needsYouHasMore={false}
      failedCount={undefined}
      failedTruncated={false}
      historyCount={0}
      {...overrides}
    />,
  );
}

afterEach(() => cleanup());

describe("InboxTabStrip", () => {
  it("renders exactly three tabs, Needs you, Failed, then History (AC .1)", () => {
    renderStrip();
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(3);
    expect(tabs[0].textContent).toContain("Needs you");
    expect(tabs[1].textContent).toContain("Failed");
    expect(tabs[2].textContent).toContain("History");
  });

  it("calls onSelectTab with the clicked tab's value", () => {
    const onSelectTab = vi.fn();
    renderStrip({ onSelectTab });
    // Radix TabsTrigger switches tabs on mousedown, not click (@radix-ui/react-tabs).
    fireEvent.mouseDown(screen.getByRole("tab", { name: /Failed/ }));
    expect(onSelectTab).toHaveBeenCalledWith("failed");
  });

  it("renders no failed badge before a response is known (AC .16)", () => {
    renderStrip();
    expect(screen.queryByTestId(FAILED_BADGE_TESTID)).toBeNull();
  });

  it("renders no failed badge when the known count is zero (AC .16)", () => {
    renderStrip({ failedCount: 0 });
    expect(screen.queryByTestId(FAILED_BADGE_TESTID)).toBeNull();
  });

  it("renders the failed badge once a non-zero count is known", () => {
    renderStrip({ failedCount: 3 });
    expect(screen.getByTestId(FAILED_BADGE_TESTID).textContent).toBe("3");
  });

  it("renders a capped indicator when the failed page is truncated (AC .17)", () => {
    renderStrip({ failedCount: 200, failedTruncated: true });
    expect(screen.getByTestId(FAILED_BADGE_TESTID).textContent).toBe("200+");
  });

  it("stays visible while the other tab is selected (AC .16)", () => {
    renderStrip({ selectedTab: "failed", failedCount: 2 });
    expect(screen.getByTestId(FAILED_BADGE_TESTID)).not.toBeNull();
  });

  it("renders no history badge when the count is zero (AC .16)", () => {
    renderStrip({ historyCount: 0 });
    expect(screen.queryByTestId(HISTORY_BADGE_TESTID)).toBeNull();
  });

  it("renders the history badge once a non-zero count is known", () => {
    renderStrip({ historyCount: 3 });
    expect(screen.getByTestId(HISTORY_BADGE_TESTID).textContent).toBe("3");
  });

  it("carries a coarse-pointer/mobile 44px touch-target floor on every tab trigger", () => {
    renderStrip();
    for (const tab of screen.getAllByRole("tab")) {
      expect(tab.className).toContain("min-h-11");
    }
  });

  it("connects a tab trigger to its content panel", () => {
    renderStrip({
      children: <TabsContent value="needs-you">Needs you content</TabsContent>,
    });

    const tab = screen.getByRole("tab", { name: /Needs you/ });
    const panel = screen.getByRole("tabpanel");
    expect(tab.getAttribute("aria-controls")).toBe(panel.getAttribute("id"));
    expect(panel.getAttribute("aria-labelledby")).toBe(tab.getAttribute("id"));
  });
});
