import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { InstallReview } from "@/lib/api/domains/canvas-distribution-api";

const review: InstallReview = {
  preparation_id: "preparation-1",
  workspace_id: "workspace-b",
  metadata: {
    package_id: "canvas-one",
    version: "1.0.0",
    display_name: "Canvas One",
    description: "A shared canvas",
    author: "Author One",
    license: "Apache-2.0",
    source_mode: "project",
    min_kandev_version: "0.94.0",
    repo_url: "https://example.test/canvas-one",
  },
  sha256: "package-digest",
  archive_sha256: "archive-digest",
  permissions: { reads: [], writes: [], events: [], external_origins: [], shared_state: false },
  origin_kind: "url",
  expires_at: "2026-09-11T12:00:00Z",
};

const install = {
  review,
  result: null,
  loading: false,
  error: null,
  prepareUrl: vi.fn(),
  prepareCatalog: vi.fn(),
  prepareUpload: vi.fn(),
  confirm: vi.fn(),
  cancel: vi.fn(),
  reset: vi.fn(),
  invalidate: vi.fn(),
};

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
vi.mock("@/hooks/domains/canvas/use-canvas-install", () => ({
  useCanvasInstall: () => install,
}));
vi.mock("./marketplace-preview-gallery", () => ({ MarketplacePreviewGallery: () => null }));
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

import { CanvasInstallDialog } from "./canvas-install-dialog";

afterEach(() => cleanup());

describe("CanvasInstallDialog", () => {
  it("shows inspected metadata, destination, and empty permission groups", () => {
    render(
      <CanvasInstallDialog
        open
        onOpenChange={vi.fn()}
        workspaceId="workspace-b"
        workspaceName="Workspace B"
        entry={null}
      />,
    );

    expect(screen.getByText("A shared canvas")).toBeTruthy();
    expect(screen.getByText("Author One")).toBeTruthy();
    expect(screen.getByText("Apache-2.0")).toBeTruthy();
    expect(screen.getByText("0.94.0")).toBeTruthy();
    expect(screen.getByText(/Workspace B \(workspace-b\)/)).toBeTruthy();
    expect(screen.getAllByText("plugins:none")).toHaveLength(5);
  });
});
