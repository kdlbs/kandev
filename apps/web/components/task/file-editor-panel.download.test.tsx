import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

const triggerFileDownload = vi.fn();

vi.mock("@/lib/utils/file-download", () => ({
  triggerFileDownload: (...args: unknown[]) => triggerFileDownload(...args),
  fileBasename: (p: string) => p.split("/").pop() ?? p,
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => <span data-testid="external-vcs-file-link" />,
  useExternalVcsFileStatus: () => null,
}));

import { FileViewerDownloadButton } from "./file-viewer-header";
import { FileBinaryViewer } from "./file-binary-viewer";
import { FileImageViewer } from "./file-image-viewer";

const DOWNLOAD_LABEL = "Download file";

afterEach(() => {
  cleanup();
  triggerFileDownload.mockReset();
});

describe("FileViewerDownloadButton", () => {
  it("renders nothing without a handler", () => {
    render(
      <TooltipProvider>
        <FileViewerDownloadButton />
      </TooltipProvider>,
    );
    expect(screen.queryByRole("button", { name: DOWNLOAD_LABEL })).toBeNull();
  });

  it("invokes the handler when activated", () => {
    const onDownload = vi.fn();
    render(
      <TooltipProvider>
        <FileViewerDownloadButton onDownload={onDownload} />
      </TooltipProvider>,
    );
    screen.getByRole("button", { name: DOWNLOAD_LABEL }).click();
    expect(onDownload).toHaveBeenCalledTimes(1);
  });
});

describe("viewer headerActions contract", () => {
  it("surfaces the download control on the unpreviewable-file screen", () => {
    const onDownload = vi.fn();
    render(
      <TooltipProvider>
        <FileBinaryViewer
          path="assets/bundle.zip"
          headerActions={<FileViewerDownloadButton onDownload={onDownload} />}
        />
      </TooltipProvider>,
    );

    screen.getByRole("button", { name: DOWNLOAD_LABEL }).click();
    expect(onDownload).toHaveBeenCalledTimes(1);
  });

  it("surfaces the download control on the image viewer", () => {
    const onDownload = vi.fn();
    render(
      <TooltipProvider>
        <FileImageViewer
          path="assets/logo.png"
          content="iVBORw0KGgo="
          headerActions={<FileViewerDownloadButton onDownload={onDownload} />}
        />
      </TooltipProvider>,
    );

    screen.getByRole("button", { name: DOWNLOAD_LABEL }).click();
    expect(onDownload).toHaveBeenCalledTimes(1);
  });
});

describe("binary download fidelity", () => {
  it("passes base64 content through as binary so the bytes survive", () => {
    const content = "iVBORw0KGgo=";
    const onDownload = () =>
      triggerFileDownload({ fileName: "assets/logo.png", content, isBinary: true });

    render(
      <TooltipProvider>
        <FileViewerDownloadButton onDownload={onDownload} />
      </TooltipProvider>,
    );
    screen.getByRole("button", { name: DOWNLOAD_LABEL }).click();

    expect(triggerFileDownload).toHaveBeenCalledWith({
      fileName: "assets/logo.png",
      content,
      isBinary: true,
    });
  });
});
