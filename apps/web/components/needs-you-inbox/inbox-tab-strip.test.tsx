import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { InboxTabStrip } from "./inbox-tab-strip";
import type { InboxTab } from "@/lib/failed-inbox/inbox-tab";

const FAILED_BADGE_TESTID = "inbox-tab-failed-badge";

function renderStrip(
  overrides: Partial<{
    selectedTab: InboxTab;
    onSelectTab: (tab: InboxTab) => void;
    needsYouCount: number;
    needsYouHasMore: boolean;
    failedCount: number | undefined;
    failedTruncated: boolean;
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
      {...overrides}
    />,
  );
}

afterEach(() => cleanup());

describe("InboxTabStrip", () => {
  it("renders exactly two tabs, Needs you then Failed (AC .1)", () => {
    renderStrip();
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(2);
    expect(tabs[0].textContent).toContain("Needs you");
    expect(tabs[1].textContent).toContain("Failed");
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
});
