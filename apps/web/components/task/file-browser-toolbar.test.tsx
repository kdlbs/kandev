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

    const addSources = screen.getByRole("menuitem", {
      name: /Add Repositories to workspace/i,
    });
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
    fireEvent.click(screen.getByRole("menuitem", { name: "Add Repositories to workspace" }));

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
    fireEvent.click(screen.getByRole("menuitem", { name: "Add Repositories to workspace" }));
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

const CREATE_MENU_TESTID = "files-create-menu";

describe("FileBrowserToolbar create menu", () => {
  const baseProps = {
    displayPath: "workspace",
    fullPath: "/workspace",
    copied: false,
    expandedPathsSize: 0,
    onCopyPath: vi.fn(),
    onOpenFolder: vi.fn(),
    onStartSearch: vi.fn(),
    onCollapseAll: vi.fn(),
  };

  it("keeps one-click New File when upload is unavailable", () => {
    const onStartCreate = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar {...baseProps} showCreateButton onStartCreate={onStartCreate} />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "New file" }));
    expect(onStartCreate).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(CREATE_MENU_TESTID)).toBeNull();
  });

  it("offers New File, Upload files, and Upload folder when upload is available", async () => {
    const onStartCreate = vi.fn();
    const onUploadFiles = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...baseProps}
          showCreateButton
          onStartCreate={onStartCreate}
          onUploadFiles={onUploadFiles}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.pointerDown(menuTrigger, { button: 0, ctrlKey: false });
    fireEvent.click(menuTrigger);
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "New file" })).toBeTruthy());
    expect(screen.getByRole("menuitem", { name: "Upload files" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Upload folder" })).toBeTruthy();
  });

  it("still begins inline creation from the menu, unchanged", async () => {
    const onStartCreate = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...baseProps}
          showCreateButton
          onStartCreate={onStartCreate}
          onUploadFiles={vi.fn()}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.pointerDown(menuTrigger, { button: 0, ctrlKey: false });
    fireEvent.click(menuTrigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: "New file" }));
    await waitFor(() => expect(onStartCreate).toHaveBeenCalledTimes(1));
  });

  it("requests the right picker for each upload item", async () => {
    const onUploadFiles = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...baseProps}
          showCreateButton
          onStartCreate={vi.fn()}
          onUploadFiles={onUploadFiles}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.pointerDown(menuTrigger, { button: 0, ctrlKey: false });
    fireEvent.click(menuTrigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: "Upload folder" }));
    await waitFor(() => expect(onUploadFiles).toHaveBeenCalledWith("folder"));

    onUploadFiles.mockClear();
    fireEvent.pointerDown(menuTrigger, { button: 0, ctrlKey: false });
    fireEvent.click(menuTrigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: "Upload files" }));
    await waitFor(() => expect(onUploadFiles).toHaveBeenCalledWith("files"));
  });
});
