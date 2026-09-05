import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { registerCanvasesHandlers } from "@/lib/ws/handlers/canvases";

const {
  mockGetCanvas,
  mockGetCanvasRuntime,
  mockListTaskCanvases,
  mockListWorkspaceCanvases,
  mockPush,
  mockIsMobile,
} = vi.hoisted(() => ({
  mockGetCanvas: vi.fn(),
  mockGetCanvasRuntime: vi.fn(),
  mockListTaskCanvases: vi.fn(),
  mockListWorkspaceCanvases: vi.fn(),
  mockPush: vi.fn(),
  mockIsMobile: { value: false },
}));

const FRAME_TEST_ID = "canvas-frame";
const RUNTIME_URL_ATTRIBUTE = "data-runtime-url";

vi.mock("@/lib/api/domains/canvas-api", () => ({
  canvasHref: (canvasId: string) => `/canvases/${canvasId}`,
  getCanvas: mockGetCanvas,
  getCanvasRuntime: mockGetCanvasRuntime,
  listTaskCanvases: mockListTaskCanvases,
  listWorkspaceCanvases: mockListWorkspaceCanvases,
  startCanvasEdit: vi.fn(),
}));

vi.mock("@/components/page-shell", () => ({
  PageShell: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/plugins/canvas-page", () => ({
  CanvasPage: ({ runtimeUrl, onError }: { runtimeUrl?: string; onError?: () => void }) => (
    <button
      type="button"
      data-testid={FRAME_TEST_ID}
      data-runtime-url={runtimeUrl ?? ""}
      onClick={onError}
    >
      frame
    </button>
  ),
}));

vi.mock("@/components/settings/canvas-lifecycle-dialogs", () => ({
  CanvasPromotionDialog: () => null,
  CanvasReleaseDialog: () => null,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mockIsMobile.value }),
}));

vi.mock("@/components/task/mobile/mobile-picker-sheet", () => ({
  MobilePickerSheet: ({
    children,
    open,
    contentTestId,
  }: {
    children: ReactNode;
    open: boolean;
    contentTestId?: string;
  }) => (open ? <div data-testid={contentTestId}>{children}</div> : null),
}));

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: mockPush }),
}));

import { CanvasHostRoute } from "./canvas-host-route";

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "plugin-1",
  workspace_id: "workspace-1",
  task_id: "task-1",
  scope_kind: "task",
  title: "Task canvas",
  status: "active",
  active_release_id: "release-1",
  active_release_status: "valid",
};

beforeEach(() => {
  mockGetCanvas.mockReset().mockResolvedValue(canvas);
  mockGetCanvasRuntime
    .mockReset()
    .mockResolvedValueOnce({
      runtime_url: "/runtime/old",
      release_id: "release-1",
      expires_in_seconds: 900,
    })
    .mockResolvedValueOnce({
      runtime_url: "/runtime/renewed",
      release_id: "release-1",
      expires_in_seconds: 900,
    });
  mockListTaskCanvases.mockReset().mockResolvedValue({ canvases: [canvas] });
  mockListWorkspaceCanvases.mockReset().mockResolvedValue({ canvases: [] });
  mockIsMobile.value = false;
  mockPush.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("CanvasHostRoute runtime recovery", () => {
  it("renews the capability URL for the same active release after a frame failure", async () => {
    render(<CanvasHostRoute canvasId="canvas-1" />);

    await waitFor(() =>
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/old",
      ),
    );

    fireEvent.click(screen.getByTestId(FRAME_TEST_ID));

    await waitFor(() => {
      expect(mockGetCanvasRuntime).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/renewed",
      );
    });
  });

  it("renews the capability URL before its expiry", async () => {
    mockGetCanvasRuntime
      .mockReset()
      .mockResolvedValueOnce({
        runtime_url: "/runtime/expiring",
        release_id: "release-1",
        expires_in_seconds: 30,
      })
      .mockResolvedValueOnce({
        runtime_url: "/runtime/refreshed",
        release_id: "release-1",
        expires_in_seconds: 900,
      });

    render(<CanvasHostRoute canvasId="canvas-1" />);
    await waitFor(() =>
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/expiring",
      ),
    );

    await waitFor(() => {
      expect(mockGetCanvasRuntime).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/refreshed",
      );
    });
  });

  it("refreshes the visible host when a canvas lifecycle event arrives", async () => {
    const updated = { ...canvas, active_release_id: "release-2" };
    mockGetCanvas.mockReset().mockResolvedValueOnce(canvas).mockResolvedValueOnce(updated);
    mockGetCanvasRuntime
      .mockReset()
      .mockResolvedValueOnce({
        runtime_url: "/runtime/release-1",
        release_id: "release-1",
        expires_in_seconds: 900,
      })
      .mockResolvedValueOnce({
        runtime_url: "/runtime/release-2",
        release_id: "release-2",
        expires_in_seconds: 900,
      });

    render(<CanvasHostRoute canvasId="canvas-1" />);
    await waitFor(() =>
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/release-1",
      ),
    );

    registerCanvasesHandlers({} as never)["canvas.release.activated"]?.({} as never);

    await waitFor(() => {
      expect(mockGetCanvas).toHaveBeenCalledTimes(2);
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/release-2",
      );
    });
  });

  it("lets a mobile focused host switch to another applicable canvas", async () => {
    mockIsMobile.value = true;
    const otherCanvas = {
      ...canvas,
      id: "canvas-2",
      title: "Another task canvas",
    };
    mockListTaskCanvases.mockResolvedValue({ canvases: [canvas, otherCanvas] });

    render(<CanvasHostRoute canvasId="canvas-1" />);

    await waitFor(() => expect(screen.getByTestId(FRAME_TEST_ID)).toBeTruthy());
    fireEvent.click(screen.getByTestId("canvas-mobile-actions"));

    const other = await screen.findByTestId("canvas-mobile-picker-item-canvas-2");
    expect(other.textContent).toContain("Another task canvas");
    fireEvent.click(other);

    expect(mockPush).toHaveBeenCalledWith("/canvases/canvas-2");
  });
});
