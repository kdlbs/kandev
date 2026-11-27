import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const replaceMock = vi.fn();
let search = "";

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/settings/workspaces/w/coordinators/c",
  useSearchParams: () => new URLSearchParams(search),
  useRouter: () => ({ replace: replaceMock, push: vi.fn() }),
}));

import { SectionsRow } from "./sections-row";

const ENTRIES = [
  {
    slug: "identity",
    label: "Identity",
    help: "Identity help",
    render: () => <p>identity body</p>,
  },
  { slug: "goal", label: "Goal", help: "Goal help", render: () => <p>goal body</p> },
];

beforeEach(() => {
  replaceMock.mockReset();
  search = "";
  window.history.replaceState({}, "", "/settings/workspaces/w/coordinators/c");
});
afterEach(cleanup);

describe("SectionsRow", () => {
  it("shows the entries in order with the default section's help", () => {
    render(<SectionsRow entries={ENTRIES} />);
    const tabs = screen.getAllByRole("tab").map((tab) => tab.textContent);
    expect(tabs).toEqual(["Identity", "Goal"]);
    expect(screen.getByTestId("coordinator-section-help").textContent).toBe("Identity help");
  });

  it("opens the section named in the address and falls back to Identity for an unknown one", () => {
    search = "section=goal";
    const { unmount } = render(<SectionsRow entries={ENTRIES} />);
    expect(screen.getByTestId("coordinator-section-help").textContent).toBe("Goal help");
    unmount();
    search = "section=watches";
    render(<SectionsRow entries={ENTRIES} />);
    expect(screen.getByTestId("coordinator-section-help").textContent).toBe("Identity help");
  });

  it("replaces the address on selection, without pushing history", () => {
    render(<SectionsRow entries={ENTRIES} />);
    const goalTab = screen.getByRole("tab", { name: "Goal" });
    fireEvent.mouseDown(goalTab);
    fireEvent.click(goalTab);
    fireEvent.keyDown(goalTab, { key: "Enter" });
    expect(replaceMock).toHaveBeenCalledWith("/settings/workspaces/w/coordinators/c?section=goal", {
      scroll: false,
    });
  });
});
