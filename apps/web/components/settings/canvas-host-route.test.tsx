import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent } from "@kandev/ui/dropdown-menu";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { registerCanvasesHandlers } from "@/lib/ws/handlers/canvases";

const {
  mockGetCanvas,
  mockGetCanvasRuntime,
  mockListTaskCanvases,
  mockListWorkspaceCanvases,
  mockPush,
  mockIsMobile,
  mockRenderPageShellOverflow,
} = vi.hoisted(() => ({
  mockGetCanvas: vi.fn(),
  mockGetCanvasRuntime: vi.fn(),
  mockListTaskCanvases: vi.fn(),
  mockListWorkspaceCanvases: vi.fn(),
  mockPush: vi.fn(),
  mockIsMobile: { value: false },
  mockRenderPageShellOverflow: { value: false },
}));

const FRAME_TEST_ID = "canvas-frame";
const RUNTIME_URL_ATTRIBUTE = "data-runtime-url";

vi.mock("@/lib/api/domains/canvas-api", () => ({
  canvasHref: (canvasId: string) => `/canvases/${canvasId}`,
  canvasDataScope: (value: Canvas) => value.data_scope_kind ?? value.scope_kind,
  canvasCanEnableWorkspaceData: (value: Canvas) =>
    value.scope_kind === "task" &&
    (value.data_scope_kind ?? value.scope_kind) === "task" &&
    value.status === "active" &&
    value.active_release_status === "valid",
  getCanvas: mockGetCanvas,
  getCanvasRuntime: mockGetCanvasRuntime,
  listTaskCanvases: mockListTaskCanvases,
  listWorkspaceCanvases: mockListWorkspaceCanvases,
  startCanvasEdit: vi.fn(),
}));

vi.mock("@/components/page-shell", () => ({
  PageShell: ({
    actions,
    children,
    overflowMenuItems,
    subtitle,
    titleSlot,
    topbarTestId,
  }: {
    actions?: ReactNode;
    children: ReactNode;
    overflowMenuItems?: ReactNode;
    subtitle?: string;
    titleSlot?: ReactNode;
    topbarTestId?: string;
  }) => (
    <div>
      <div data-testid={topbarTestId}>
        {titleSlot}
        {subtitle}
        {mockIsMobile.value ? actions : null}
      </div>
      {overflowMenuItems && mockRenderPageShellOverflow.value ? (
        <DropdownMenu open>
          <DropdownMenuContent data-testid="page-shell-overflow">
            {overflowMenuItems}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
      {children}
    </div>
  ),
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
  CanvasWorkspaceDataDialog: () => null,
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

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({ auth: { mode: "disabled", user: null } }),
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
  mockRenderPageShellOverflow.value = false;
  mockPush.mockReset();
});

afterEach(() => {
  cleanup();
});

describe("CanvasHostRoute runtime recovery", () => {
  it("exposes retry after a frame failure and renews the capability URL", async () => {
    render(<CanvasHostRoute canvasId="canvas-1" />);

    await waitFor(() =>
      expect(screen.getByTestId(FRAME_TEST_ID).getAttribute(RUNTIME_URL_ATTRIBUTE)).toBe(
        "/runtime/old",
      ),
    );

    fireEvent.click(screen.getByTestId(FRAME_TEST_ID));

    await waitFor(() =>
      expect(screen.getByTestId("canvas-host-state").textContent).toContain("unavailable"),
    );
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));

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

  it("passes raw desktop overflow items to the standalone page shell", async () => {
    mockRenderPageShellOverflow.value = true;
    render(<CanvasHostRoute canvasId="canvas-1" />);

    await waitFor(() => expect(screen.getByTestId(FRAME_TEST_ID)).toBeTruthy());

    expect(screen.getByRole("menuitem", { name: "Releases and permissions" })).toBeTruthy();
    expect(screen.queryByTestId("panel-header-overflow")).toBeNull();
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
});

describe("CanvasHostRoute mobile data scope", () => {
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

  it("shows the data scope beside the title on mobile", async () => {
    mockIsMobile.value = true;
    mockGetCanvas.mockReset().mockResolvedValue({ ...canvas, data_scope_kind: "workspace" });

    render(<CanvasHostRoute canvasId="canvas-1" />);

    expect((await screen.findByTestId("canvas-data-scope")).textContent).toBe("Workspace data");
  });
});
