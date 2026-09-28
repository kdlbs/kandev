import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import type { ExportReview } from "@/lib/api/domains/canvas-distribution-api";

const prepare = vi.fn();
const getDefaults = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/canvas-distribution-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/canvas-distribution-api")>()),
  getCanvasExportDefaults: getDefaults,
}));
const share = {
  review: null as ExportReview | null,
  loading: false,
  error: null,
  prepare,
  download: vi.fn(),
  cancel: vi.fn(),
  reset: vi.fn(),
  invalidate: vi.fn(),
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { size?: string | number }) =>
      options?.size === undefined ? key : `${key}:${options.size}`,
  }),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@/hooks/domains/canvas/use-canvas-share", () => ({
  useCanvasShare: () => share,
}));
vi.mock("./canvas-share-help", () => ({ CanvasShareHelp: () => null }));
vi.mock("@kandev/ui/dialog", () => ({
  Dialog: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DialogContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DialogDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DialogFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DialogHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DialogTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));
vi.mock("@kandev/ui/drawer", () => ({
  Drawer: ({ children, open }: { children?: ReactNode; open?: boolean }) =>
    open ? <>{children}</> : null,
  DrawerContent: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  DrawerDescription: ({ children }: { children?: ReactNode }) => <p>{children}</p>,
  DrawerFooter: ({ children }: { children?: ReactNode }) => <footer>{children}</footer>,
  DrawerHeader: ({ children }: { children?: ReactNode }) => <header>{children}</header>,
  DrawerTitle: ({ children }: { children?: ReactNode }) => <h2>{children}</h2>,
}));

import { CanvasShareDialog } from "./canvas-share-dialog";

afterEach(() => cleanup());

const PACKAGE_ID = "canvas-one";
const DISPLAY_NAME = "Canvas One";
const PREPARE_LABEL = "canvases:prepareDownloads";

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "canvas-1",
  workspace_id: "workspace-1",
  scope_kind: "workspace",
  title: DISPLAY_NAME,
  status: "active",
  active_release_id: "release-1",
  active_release_status: "valid",
  active_release: {
    id: "release-1",
    validation_status: "valid",
    package_id: PACKAGE_ID,
    version: "1.0.0",
    display_name: DISPLAY_NAME,
    description: "A portable canvas",
    author: "Author",
    source_mode: "static",
    min_kandev_version: "0.94.0",
  },
} as Canvas;

beforeEach(() => {
  getDefaults.mockReset().mockResolvedValue({
    expected_release_id: "release-1",
    metadata: {
      package_id: PACKAGE_ID,
      version: "1.0.0",
      display_name: DISPLAY_NAME,
      description: "A portable canvas",
      author: "Author",
      source_mode: "static",
      min_kandev_version: "0.94.0",
    },
    missing_required: ["license"],
  });
  prepare.mockReset().mockResolvedValue(null);
  share.cancel.mockReset();
  share.reset.mockReset();
  share.invalidate.mockReset();
  share.download.mockReset();
  share.review = null;
});

describe("CanvasShareDialog", () => {
  it("submits release defaults with an explicit user supplied license", async () => {
    render(<CanvasShareDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    const gaps = await screen.findByTestId("canvas-share-required-gaps");
    fireEvent.change(within(gaps).getByLabelText("canvases:license"), {
      target: { value: "MIT" },
    });
    fireEvent.click(screen.getByRole("button", { name: PREPARE_LABEL }));

    await waitFor(() =>
      expect(prepare).toHaveBeenCalledWith(
        expect.objectContaining({
          package_id: PACKAGE_ID,
          version: "1.0.0",
          author: "Author",
          license: "MIT",
          source_mode: "static",
          min_kandev_version: "0.94.0",
        }),
      ),
    );
  });

  it("lists the retained export files with their sizes", () => {
    share.review = {
      preparation_id: "preparation-1",
      canvas_id: "canvas-1",
      workspace_id: "workspace-1",
      release_id: "release-1",
      metadata: { package_id: PACKAGE_ID, version: "1.0.0" },
      sha256: "digest",
      files: [{ path: "assets/logo.svg", bytes: 12 }],
      bundle_bytes: 20,
      source_bytes: 18,
      expires_at: "2026-09-11T12:00:00Z",
      bundle_download: "",
      source_download: "",
    };
    share.download.mockReset().mockResolvedValue(undefined);

    render(<CanvasShareDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    expect(screen.getByText("assets/logo.svg")).toBeTruthy();
    expect(screen.getByText("canvases:downloadSize:12")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "canvases:downloadBundle" }));
    fireEvent.click(screen.getByRole("button", { name: "canvases:downloadSource" }));
    expect(share.download).toHaveBeenNthCalledWith(1, "bundle");
    expect(share.download).toHaveBeenNthCalledWith(2, "source");
  });

  it("blocks preparation until release defaults load and offers retry on failure", async () => {
    getDefaults.mockRejectedValueOnce(new Error("offline"));
    render(<CanvasShareDialog canvas={canvas} open onOpenChange={vi.fn()} />);
    await screen.findByRole("alert");
    expect(
      (screen.getByRole("button", { name: PREPARE_LABEL }) as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "canvases:retry" }));
    await screen.findByTestId("canvas-share-package-details");
    expect(
      (screen.getByRole("button", { name: PREPARE_LABEL }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("requires an explicit source mode when the release has no usable default", async () => {
    getDefaults.mockResolvedValueOnce({
      expected_release_id: "release-1",
      metadata: {
        package_id: PACKAGE_ID,
        version: "1.0.0",
        display_name: DISPLAY_NAME,
        description: "A portable canvas",
        author: "Author",
        license: "MIT",
        source_mode: "",
        min_kandev_version: "0.95.0",
      },
      missing_required: ["source_mode"],
    });
    render(<CanvasShareDialog canvas={canvas} open onOpenChange={vi.fn()} />);
    const gaps = await screen.findByTestId("canvas-share-required-gaps");
    const sourceMode = within(gaps).getByRole("combobox", { name: "canvases:sourceMode" });
    fireEvent.click(screen.getByRole("button", { name: PREPARE_LABEL }));
    expect(prepare).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement).toBe(sourceMode));
    fireEvent.click(sourceMode);
    fireEvent.click(screen.getByRole("option", { name: "canvases:sourceModeProject" }));
    fireEvent.click(screen.getByRole("button", { name: PREPARE_LABEL }));
    await waitFor(() =>
      expect(prepare).toHaveBeenCalledWith(expect.objectContaining({ source_mode: "project" })),
    );
  });
});
