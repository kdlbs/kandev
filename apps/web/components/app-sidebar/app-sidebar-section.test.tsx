import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IconCircleDot } from "@tabler/icons-react";
import { TooltipProvider } from "@kandev/ui/tooltip";

const storeState = {
  appSidebar: {
    sectionExpanded: { tasks: true } as Record<string, boolean>,
  },
  toggleAppSidebarSection: vi.fn(),
  setAppSidebarCollapsed: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { AppSidebarSection } from "./app-sidebar-section";

const CHILDREN_TESTID = "section-children";

function renderSection(props: { collapsed: boolean; grow?: boolean }) {
  return render(
    <TooltipProvider>
      <AppSidebarSection id="tasks" label="Tasks" icon={IconCircleDot} {...props}>
        <div data-testid={CHILDREN_TESTID}>children</div>
      </AppSidebarSection>
    </TooltipProvider>,
  );
}

describe("AppSidebarSection", () => {
  beforeEach(() => {
    storeState.appSidebar.sectionExpanded = { tasks: true };
    storeState.toggleAppSidebarSection = vi.fn();
    storeState.setAppSidebarCollapsed = vi.fn();
  });

  afterEach(() => {
    cleanup();
  });

  it("unmounts children of a non-grow section when the sidebar collapses", () => {
    renderSection({ collapsed: true });
    expect(screen.queryByTestId(CHILDREN_TESTID)).toBeNull();
    expect(screen.getByRole("button", { name: "Tasks" })).toBeTruthy();
  });

  it("keeps grow-section children mounted but hidden when the sidebar collapses", () => {
    renderSection({ collapsed: true, grow: true });
    const children = screen.getByTestId(CHILDREN_TESTID);
    expect(children.parentElement?.classList.contains("hidden")).toBe(true);
    expect(screen.getByRole("button", { name: "Tasks" })).toBeTruthy();
  });

  it("preserves the grow-section children instance across a collapse toggle (no remount)", () => {
    const { rerender } = renderSection({ collapsed: false, grow: true });
    const before = screen.getByTestId(CHILDREN_TESTID);
    expect(before.parentElement?.classList.contains("hidden")).toBe(false);

    rerender(
      <TooltipProvider>
        <AppSidebarSection id="tasks" label="Tasks" icon={IconCircleDot} collapsed grow>
          <div data-testid={CHILDREN_TESTID}>children</div>
        </AppSidebarSection>
      </TooltipProvider>,
    );

    const after = screen.getByTestId(CHILDREN_TESTID);
    expect(after).toBe(before);
    expect(after.parentElement?.classList.contains("hidden")).toBe(true);

    // round-trip: re-expanding should restore the visible state on the same node
    rerender(
      <TooltipProvider>
        <AppSidebarSection id="tasks" label="Tasks" icon={IconCircleDot} collapsed={false} grow>
          <div data-testid={CHILDREN_TESTID}>children</div>
        </AppSidebarSection>
      </TooltipProvider>,
    );

    const reopened = screen.getByTestId(CHILDREN_TESTID);
    expect(reopened).toBe(before);
    expect(reopened.parentElement?.classList.contains("hidden")).toBe(false);
    expect(reopened.parentElement?.classList.contains("sidebar-fade-in")).toBe(true);
  });

  it("does not render grow-section children while the section accordion is closed", () => {
    storeState.appSidebar.sectionExpanded = { tasks: false };
    renderSection({ collapsed: true, grow: true });
    expect(screen.queryByTestId(CHILDREN_TESTID)).toBeNull();
  });

  it("expands the sidebar when the collapsed rail button is clicked", () => {
    renderSection({ collapsed: true, grow: true });
    fireEvent.click(screen.getByRole("button", { name: "Tasks" }));
    expect(storeState.setAppSidebarCollapsed).toHaveBeenCalledWith(false);
    expect(storeState.toggleAppSidebarSection).not.toHaveBeenCalled();
  });

  it("also re-opens the section accordion when expanding via the rail button", () => {
    storeState.appSidebar.sectionExpanded = { tasks: false };
    renderSection({ collapsed: true, grow: true });
    fireEvent.click(screen.getByRole("button", { name: "Tasks" }));
    expect(storeState.setAppSidebarCollapsed).toHaveBeenCalledWith(false);
    expect(storeState.toggleAppSidebarSection).toHaveBeenCalledWith("tasks");
  });
});
