import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";

const COPY: Record<string, string> = {
  "canvases:editCanvas": "Edit canvas",
  "canvases:releasesAndPermissions": "Releases and permissions",
  "canvases:promoteCanvas": "Promote canvas",
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
}));

import {
  CanvasDesktopActions,
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

describe("canvas host chrome", () => {
  it("keeps state content in the body and leaves the shared header for title/actions", () => {
    render(
      <>
        <CanvasHostHeader
          title="Task canvas"
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
