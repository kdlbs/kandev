import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { FileBrowserToolbar } from "./file-browser-toolbar";

let isMobile = false;

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile }),
}));

afterEach(() => {
  cleanup();
  isMobile = false;
});

describe("FileBrowserToolbar workspace actions", () => {
  it("keeps Open workspace folder available while Add sources explains why it is unavailable", () => {
    const onAddSources = vi.fn();
    const onOpenFolder = vi.fn();

    render(
      <TooltipProvider>
        <FileBrowserToolbar
          displayPath="workspace"
          fullPath="/workspace"
          copied={false}
          expandedPathsSize={0}
          onCopyPath={vi.fn()}
          onOpenFolder={onOpenFolder}
          onStartSearch={vi.fn()}
          onCollapseAll={vi.fn()}
          showCreateButton={false}
          onAddSources={onAddSources}
          addSourcesDisabledReason="Wait for the active agent turn to finish"
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Workspace actions" });
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);

    const addSources = screen.getByRole("menuitem", { name: /Add sources/i });
    expect(addSources.hasAttribute("data-disabled")).toBe(true);
    expect(addSources.className).toContain("min-h-11");
    expect(screen.getByText("Wait for the active agent turn to finish")).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: "Open workspace folder" }));
    expect(onOpenFolder).toHaveBeenCalledOnce();
    expect(onAddSources).not.toHaveBeenCalled();
  });

  it("opens Add sources only after the menu closes and uses the combined trigger as its opener", async () => {
    const onAddSources = vi.fn();
    const openerRef = { current: null as HTMLButtonElement | null };

    render(
      <TooltipProvider>
        <FileBrowserToolbar
          displayPath="workspace"
          fullPath="/workspace"
          copied={false}
          expandedPathsSize={0}
          onCopyPath={vi.fn()}
          onOpenFolder={vi.fn()}
          onStartSearch={vi.fn()}
          onCollapseAll={vi.fn()}
          showCreateButton={false}
          onAddSources={onAddSources}
          addSourcesButtonRef={openerRef}
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Workspace actions" });
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: "Add sources" }));

    await waitFor(() => expect(onAddSources).toHaveBeenCalledWith(trigger));
    expect(openerRef.current).toBe(trigger);
  });

  it("restores the mobile trigger after the source Drawer finishes closing", async () => {
    isMobile = true;
    const onAddSources = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          displayPath="workspace"
          fullPath="/workspace"
          copied={false}
          expandedPathsSize={0}
          onCopyPath={vi.fn()}
          onOpenFolder={vi.fn()}
          onStartSearch={vi.fn()}
          onCollapseAll={vi.fn()}
          showCreateButton={false}
          onAddSources={onAddSources}
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByRole("button", { name: "Workspace actions" });
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: "Add sources" }));
    await waitFor(() => expect(onAddSources).toHaveBeenCalledOnce());

    const drawer = document.createElement("div");
    drawer.dataset.testid = "add-workspace-sources-drawer";
    drawer.dataset.state = "closed";
    document.body.append(drawer);
    fireEvent.animationEnd(drawer);

    await waitFor(() => expect(document.activeElement).toBe(trigger));
    drawer.remove();
  });
});
