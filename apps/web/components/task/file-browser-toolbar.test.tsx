import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { FileBrowserToolbar } from "./file-browser-toolbar";
import { InlineFileInput } from "./inline-file-input";

let isMobile = false;
const ADD_SOURCES_LABEL = "Add repositories or folders";
const ADD_SOURCES_PATTERN = /Add repositories or folders/i;
const NEW_FILE_LABEL = "New file";
const UPLOAD_FILES_LABEL = "Upload files";
const UPLOAD_FOLDER_LABEL = "Upload folder";
const ACTIVE_TURN_DISABLED_REASON = "Wait for the active turn to finish";
const ACTIVE_AGENT_TURN_DISABLED_REASON = "Wait for the active agent turn to finish";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile }),
}));

afterEach(() => {
  cleanup();
  isMobile = false;
});

describe("FileBrowserToolbar workspace actions", () => {
  it("keeps Open workspace folder in overflow after source attachment moves to +", async () => {
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
          showCreateButton
          onStartCreate={vi.fn()}
          onAddSources={onAddSources}
          addSourcesDisabledReason={ACTIVE_AGENT_TURN_DISABLED_REASON}
        />
      </TooltipProvider>,
    );

    const createTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.pointerDown(createTrigger, { button: 0, ctrlKey: false });
    fireEvent.click(createTrigger);
    const addSources = await screen.findByRole("menuitem", {
      name: ADD_SOURCES_PATTERN,
    });
    expect(addSources.hasAttribute("data-disabled")).toBe(true);
    expect(addSources.className).toContain("min-h-[44px]");
    expect(screen.getByText(ACTIVE_AGENT_TURN_DISABLED_REASON)).toBeTruthy();

    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    const trigger = screen.getByRole("button", { name: "Workspace actions" });
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    expect(screen.queryByRole("menuitem", { name: /Add Repositories/i })).toBeNull();
    fireEvent.click(screen.getByRole("menuitem", { name: "Open workspace folder" }));
    expect(onOpenFolder).toHaveBeenCalledOnce();
    expect(onAddSources).not.toHaveBeenCalled();
  });
});

describe("FileBrowserToolbar source opener", () => {
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
          showCreateButton
          onStartCreate={vi.fn()}
          onAddSources={onAddSources}
          addSourcesButtonRef={openerRef}
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: ADD_SOURCES_LABEL }));

    await waitFor(() => expect(onAddSources).toHaveBeenCalledWith(trigger));
    expect(openerRef.current).toBe(trigger);
  });
});

describe("FileBrowserToolbar mobile source opener", () => {
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
          showCreateButton
          onStartCreate={vi.fn()}
          onAddSources={onAddSources}
        />
      </TooltipProvider>,
    );

    const trigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByRole("menuitem", { name: ADD_SOURCES_LABEL }));
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
const createMenuBaseProps = {
  displayPath: "workspace",
  fullPath: "/workspace",
  copied: false,
  expandedPathsSize: 0,
  onCopyPath: vi.fn(),
  onOpenFolder: vi.fn(),
  onStartSearch: vi.fn(),
  onCollapseAll: vi.fn(),
};

async function openCreateMenu(trigger: HTMLElement) {
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
  fireEvent.click(trigger);
  return screen.findByRole("menuitem", { name: NEW_FILE_LABEL });
}

function CreateMenuInlineInputHarness() {
  const [creating, setCreating] = useState(false);

  return (
    <TooltipProvider>
      <FileBrowserToolbar
        {...createMenuBaseProps}
        showCreateButton
        onStartCreate={() => setCreating(true)}
        onUploadFiles={vi.fn()}
      />
      {creating && (
        <InlineFileInput depth={0} onSubmit={vi.fn()} onCancel={() => setCreating(false)} />
      )}
    </TooltipProvider>
  );
}

describe("FileBrowserToolbar source menu", () => {
  it("offers source attachment from + when uploads are unavailable", async () => {
    const onStartCreate = vi.fn();
    const onAddSources = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...createMenuBaseProps}
          showCreateButton
          onStartCreate={onStartCreate}
          onAddSources={onAddSources}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    await openCreateMenu(menuTrigger);
    expect(screen.getByRole("menuitem", { name: ADD_SOURCES_LABEL })).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: ADD_SOURCES_LABEL }));

    await waitFor(() => expect(onAddSources).toHaveBeenCalledWith(menuTrigger));
    expect(screen.queryByRole("menuitem", { name: "Open workspace folder" })).toBeNull();
    expect(onStartCreate).not.toHaveBeenCalled();
  });

  it("keeps source attachment visible with its disabled reason", async () => {
    const onAddSources = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...createMenuBaseProps}
          showCreateButton
          onStartCreate={vi.fn()}
          onAddSources={onAddSources}
          addSourcesDisabledReason={ACTIVE_TURN_DISABLED_REASON}
        />
      </TooltipProvider>,
    );

    await openCreateMenu(screen.getByTestId(CREATE_MENU_TESTID));
    const addSources = screen.getByRole("menuitem", {
      name: ADD_SOURCES_PATTERN,
    });
    expect(addSources.hasAttribute("data-disabled")).toBe(true);
    expect(screen.getByText(ACTIVE_TURN_DISABLED_REASON)).toBeTruthy();
    expect(onAddSources).not.toHaveBeenCalled();
  });
});

describe("FileBrowserToolbar create actions", () => {
  it("keeps one-click New File when upload is unavailable", () => {
    const onStartCreate = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...createMenuBaseProps}
          showCreateButton
          onStartCreate={onStartCreate}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: NEW_FILE_LABEL }));
    expect(onStartCreate).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId(CREATE_MENU_TESTID)).toBeNull();
  });

  it("offers New File, Upload files, and Upload folder when upload is available", async () => {
    const onStartCreate = vi.fn();
    const onUploadFiles = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...createMenuBaseProps}
          showCreateButton
          onStartCreate={onStartCreate}
          onUploadFiles={onUploadFiles}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    await openCreateMenu(menuTrigger);
    expect(screen.getByRole("menuitem", { name: UPLOAD_FILES_LABEL })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: UPLOAD_FOLDER_LABEL })).toBeTruthy();
  });
});

describe("FileBrowserToolbar create menu focus", () => {
  it("keeps inline creation focused after the create menu closes", async () => {
    // @covers AC-UI-WORKSPACE-FILE-TRANSFER-001.1
    render(<CreateMenuInlineInputHarness />);

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    fireEvent.click(await openCreateMenu(menuTrigger));
    await waitFor(() =>
      expect(screen.queryByRole("menuitem", { name: NEW_FILE_LABEL })).toBeNull(),
    );

    const input = await screen.findByPlaceholderText("filename...");
    await waitFor(() => expect(document.activeElement).toBe(input));

    fireEvent.keyDown(input, { key: "Escape" });
    await waitFor(() => expect(screen.queryByPlaceholderText("filename...")).toBeNull());
    await openCreateMenu(menuTrigger);
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() =>
      expect(screen.queryByRole("menuitem", { name: NEW_FILE_LABEL })).toBeNull(),
    );
    expect(screen.queryByPlaceholderText("filename...")).toBeNull();
  });

  it("requests the right picker for each upload item", async () => {
    const onUploadFiles = vi.fn();
    const onStartCreate = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...createMenuBaseProps}
          showCreateButton
          onStartCreate={onStartCreate}
          onUploadFiles={onUploadFiles}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    await openCreateMenu(menuTrigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: UPLOAD_FOLDER_LABEL }));
    await waitFor(() => expect(onUploadFiles).toHaveBeenCalledWith("folder"));
    await waitFor(() => expect(document.activeElement).toBe(menuTrigger));
    expect(onStartCreate).not.toHaveBeenCalled();

    onUploadFiles.mockClear();
    await openCreateMenu(menuTrigger);
    fireEvent.click(await screen.findByRole("menuitem", { name: UPLOAD_FILES_LABEL }));
    await waitFor(() => expect(onUploadFiles).toHaveBeenCalledWith("files"));
    await waitFor(() => expect(document.activeElement).toBe(menuTrigger));
    expect(onStartCreate).not.toHaveBeenCalled();
  });

  it("restores the create-menu trigger when the menu is dismissed", async () => {
    const onStartCreate = vi.fn();
    render(
      <TooltipProvider>
        <FileBrowserToolbar
          {...createMenuBaseProps}
          showCreateButton
          onStartCreate={onStartCreate}
          onUploadFiles={vi.fn()}
        />
      </TooltipProvider>,
    );

    const menuTrigger = screen.getByTestId(CREATE_MENU_TESTID);
    await openCreateMenu(menuTrigger);
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });

    await waitFor(() =>
      expect(screen.queryByRole("menuitem", { name: NEW_FILE_LABEL })).toBeNull(),
    );
    await waitFor(() => expect(document.activeElement).toBe(menuTrigger));
    expect(onStartCreate).not.toHaveBeenCalled();
  });
});
