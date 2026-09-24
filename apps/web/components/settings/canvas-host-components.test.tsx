import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent } from "@kandev/ui/dropdown-menu";
import type { Canvas } from "@/lib/api/domains/canvas-api";

const COPY: Record<string, string> = {
  "canvases:editCanvas": "Edit canvas",
  "canvases:releasesAndPermissions": "Releases and permissions",
  "canvases:promoteCanvas": "Promote canvas",
  "canvases:enableWorkspaceData": "Enable workspace data",
  "canvases:enableWorkspaceDataHelp": "Enable declared workspace data access.",
  "canvases:taskDataScope": "Task data",
  "canvases:workspaceDataScope": "Workspace data",
  "canvases:taskPlacementScope": "Task",
  "canvases:workspacePlacementScope": "Workspace",
  "canvases:canvasActions": "Canvas actions",
  "canvases:canvases": "Canvases",
  "canvases:openInNewTab": "Open in new tab",
  "canvases:editCanvasHelp": "Open the canvas authoring task.",
  "canvases:releasesAndPermissionsHelp": "Review releases and approve permissions.",
  "canvases:promoteCanvasHelp": "Make this task canvas available in workspace navigation.",
  "canvases:promoteCanvasUnavailable": "Promotion is available after a valid release.",
  "canvases:archivedCanvasActionHelp":
    "This canvas is archived. Restore it from workspace settings before changing it.",
  "canvases:disabledCanvasActionHelp": "This canvas is disabled. Enable it before changing it.",
  "canvases:shareCanvas": "Share canvas",
  "canvases:shareCanvasDescription":
    "Prepare a verified bundle or source archive for review and sharing.",
  "canvases:ready": "Ready",
  "canvases:unavailable": "Canvas unavailable",
  "canvases:unavailableDescription": "The canvas runtime is unavailable.",
  "canvases:retry": "Try again",
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => COPY[key] ?? key }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/plugins/canvas-page", () => ({ CanvasPage: () => null }));
vi.mock("./canvas-lifecycle-dialogs", () => ({
  CanvasPromotionDialog: () => null,
  CanvasReleaseDialog: () => null,
  CanvasWorkspaceDataDialog: () => null,
}));

import {
  CanvasDesktopActions,
  CanvasDesktopOverflowMenuItems,
  CanvasHostBody,
  CanvasHostHeader,
  CanvasHostStatePanel,
  MobileCanvasActions,
} from "./canvas-host-components";

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "plugin-1",
  workspace_id: "workspace-1",
  task_id: "task-1",
  scope_kind: "task",
  title: "Task canvas",
  status: "active",
  active_release_status: "pending_permission",
};

afterEach(() => cleanup());

describe("canvas host action guidance", () => {
  it("keeps disabled desktop actions keyboard-readable", () => {
    render(
      <CanvasDesktopActions
        canvas={{ ...canvas, status: "archived", scope_kind: "workspace" }}
        editing={false}
        onEdit={vi.fn()}
        onPromote={vi.fn()}
        onReleases={vi.fn()}
        onShare={vi.fn()}
      />,
    );

    const editButton = screen.getByRole("button", { name: "Edit canvas" });
    expect((editButton as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("canvas-action-edit-tooltip-trigger").getAttribute("tabindex")).toBe(
      "0",
    );
    expect(
      screen.getByText(
        "This canvas is archived. Restore it from workspace settings before changing it.",
      ),
    ).toBeTruthy();
  });

  it("describes disabled overflow actions to assistive technology", () => {
    const workspaceCanvas = {
      ...canvas,
      scope_kind: "workspace" as const,
      status: "archived" as const,
    };
    const taskCanvas = { ...canvas, active_release_status: "pending_permission" as const };

    render(
      <DropdownMenu open>
        <DropdownMenuContent>
          <CanvasDesktopOverflowMenuItems
            canvas={workspaceCanvas}
            editing={false}
            onEdit={vi.fn()}
            onPromote={vi.fn()}
            onReleases={vi.fn()}
            onShare={vi.fn()}
          />
          <CanvasDesktopOverflowMenuItems
            canvas={taskCanvas}
            editing={false}
            onEdit={vi.fn()}
            onPromote={vi.fn()}
            onReleases={vi.fn()}
            onShare={vi.fn()}
          />
        </DropdownMenuContent>
      </DropdownMenu>,
    );

    const edit = screen.getAllByRole("menuitem", { name: /Edit canvas/ })[0];
    const promote = screen.getAllByRole("menuitem", { name: /Promote canvas/ })[0];
    expect(edit.getAttribute("aria-describedby")).toBe("canvas-edit-overflow-help-canvas-1");
    expect(promote.getAttribute("aria-describedby")).toBe("canvas-promote-overflow-help-canvas-1");
    expect(screen.getByText(/archived.*changing it/i)).toBeTruthy();
    expect(screen.getByText(/valid release/i)).toBeTruthy();
  });

  it("shows lifecycle descriptions in the mobile action drawer", () => {
    render(
      <MobileCanvasActions
        canvas={canvas}
        canvases={[canvas]}
        open
        onOpenChange={vi.fn()}
        onEdit={vi.fn()}
        onPromote={vi.fn()}
        onReleases={vi.fn()}
        onShare={vi.fn()}
        onRename={vi.fn()}
        onSelectCanvas={vi.fn()}
        editing={false}
      />,
    );

    expect(screen.getByTestId("canvas-action-releases-help").textContent).toContain(
      "Review releases and approve permissions.",
    );
    expect(screen.getByTestId("canvas-action-promote-help").textContent).toContain(
      "Promotion is available after a valid release.",
    );
  });
});

describe("legacy workspace data action", () => {
  it("offers the legacy workspace data review on desktop and mobile actions", () => {
    const legacyCanvas = {
      ...canvas,
      status: "active" as const,
      active_release_status: "valid" as const,
    };
    const onEnableWorkspaceData = vi.fn();
    render(
      <>
        <CanvasDesktopActions
          canvas={legacyCanvas}
          editing={false}
          onEdit={vi.fn()}
          onPromote={vi.fn()}
          onReleases={vi.fn()}
          onShare={vi.fn()}
          onEnableWorkspaceData={onEnableWorkspaceData}
        />
        <MobileCanvasActions
          canvas={legacyCanvas}
          canvases={[legacyCanvas]}
          open
          onOpenChange={vi.fn()}
          onEdit={vi.fn()}
          onPromote={vi.fn()}
          onReleases={vi.fn()}
          onShare={vi.fn()}
          onRename={vi.fn()}
          onEnableWorkspaceData={onEnableWorkspaceData}
          onSelectCanvas={vi.fn()}
          editing={false}
        />
      </>,
    );

    const buttons = screen.getAllByRole("button", { name: "Enable workspace data" });
    expect(buttons).toHaveLength(2);
    buttons.forEach((button) => fireEvent.click(button));
    expect(onEnableWorkspaceData).toHaveBeenCalledTimes(2);
  });
});

describe("canvas host chrome", () => {
  it("keeps state content in the body and leaves the shared header for title/actions", () => {
    render(
      <>
        <CanvasHostHeader
          title="Task canvas"
          dataScopeLabel="Workspace data"
          isMobile={false}
          menuOpen={false}
          onOpenActions={vi.fn()}
          actions={<button type="button">Release actions</button>}
        />
        <CanvasHostStatePanel state="unavailable" error={null} onRetry={vi.fn()} />
      </>,
    );

    expect(screen.getByTestId("canvas-host-header").textContent).toContain("Task canvas");
    expect(screen.getByTestId("canvas-host-header").textContent).toContain("Release actions");
    expect(screen.getByTestId("canvas-data-scope").textContent).toBe("Workspace data");
    expect(screen.getByTestId("canvas-host-state").textContent).toContain("Canvas unavailable");
    expect(
      screen
        .getByTestId("canvas-host-state-panel")
        .contains(screen.getByTestId("canvas-host-state")),
    ).toBe(true);
  });

  it("announces readiness politely without adding a visible status toolbar", () => {
    render(
      <CanvasHostBody
        canvasId="canvas-1"
        title="Task canvas"
        state="ready"
        runtimeUrl="/runtime/canvas"
        error={null}
        onRuntimeReady={vi.fn()}
        onRuntimeError={vi.fn()}
        onRetry={vi.fn()}
      />,
    );

    const announcement = screen.getByTestId("canvas-host-ready-announcement");
    expect(announcement.textContent).toContain("Ready");
    expect(announcement.parentElement?.getAttribute("role")).toBe("status");
    expect(announcement.parentElement?.getAttribute("aria-live")).toBe("polite");
    expect(screen.getByTestId("canvas-host-route").textContent).not.toContain("Canvas unavailable");
  });
});
