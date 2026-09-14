import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import type { ExportReview } from "@/lib/api/domains/canvas-distribution-api";

const prepare = vi.fn();
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

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "canvas-1",
  workspace_id: "workspace-1",
  scope_kind: "workspace",
  title: "Canvas One",
  status: "active",
  active_release_id: "release-1",
  active_release_status: "valid",
  active_release: {
    id: "release-1",
    validation_status: "valid",
    package_id: "canvas-one",
    version: "1.0.0",
    display_name: "Canvas One",
    description: "A portable canvas",
    author: "Author",
    source_mode: "static",
    min_kandev_version: "0.94.0",
  },
} as Canvas;

beforeEach(() => {
  prepare.mockReset().mockResolvedValue(null);
  share.cancel.mockReset();
  share.reset.mockReset();
  share.invalidate.mockReset();
  share.download.mockReset();
  share.review = null;
});

describe("CanvasShareDialog", () => {
  it("submits editable distribution metadata, including a user supplied license", () => {
    render(<CanvasShareDialog canvas={canvas} open onOpenChange={vi.fn()} />);

    fireEvent.change(screen.getByLabelText("canvases:license"), {
      target: { value: "MIT" },
    });
    fireEvent.click(screen.getByRole("button", { name: "canvases:prepareDownloads" }));

    expect(prepare).toHaveBeenCalledWith(
      expect.objectContaining({
        package_id: "canvas-one",
        version: "1.0.0",
        author: "Author",
        license: "MIT",
        source_mode: "static",
        min_kandev_version: "0.94.0",
      }),
    );
  });

  it("lists the retained export files with their sizes", () => {
    share.review = {
      preparation_id: "preparation-1",
      canvas_id: "canvas-1",
      workspace_id: "workspace-1",
      release_id: "release-1",
      metadata: { package_id: "canvas-one", version: "1.0.0" },
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
});
