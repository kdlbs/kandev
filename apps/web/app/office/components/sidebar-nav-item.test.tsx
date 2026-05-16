import { describe, it, expect, afterEach, vi } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { IconLayoutDashboard } from "@tabler/icons-react";

// next/navigation pathname stub — not core to these assertions.
vi.mock("next/navigation", () => ({
  usePathname: () => "/office/other",
}));

import { SidebarNavItem } from "./sidebar-nav-item";

afterEach(() => cleanup());

describe("SidebarNavItem live badge", () => {
  it("does not render the live badge when liveCount is 0", () => {
    render(
      <SidebarNavItem icon={IconLayoutDashboard} label="Dashboard" href="/office" liveCount={0} />,
    );
    expect(screen.queryByText(/live/)).toBeNull();
  });

  it("does not render the live badge when liveCount is undefined", () => {
    render(<SidebarNavItem icon={IconLayoutDashboard} label="Dashboard" href="/office" />);
    expect(screen.queryByText(/live/)).toBeNull();
  });

  it("renders '1 live' (LiveAgentIndicator) when liveCount is 1", () => {
    render(
      <SidebarNavItem icon={IconLayoutDashboard} label="Dashboard" href="/office" liveCount={1} />,
    );
    expect(screen.getByText("1 live")).toBeTruthy();
  });

  it("renders '4 live' when liveCount is 4", () => {
    render(
      <SidebarNavItem icon={IconLayoutDashboard} label="Dashboard" href="/office" liveCount={4} />,
    );
    expect(screen.getByText("4 live")).toBeTruthy();
  });
});
