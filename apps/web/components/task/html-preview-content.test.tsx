import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HtmlPreviewContent } from "./html-preview-content";

const usePreviewCapture = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/use-preview-capture", () => ({ usePreviewCapture }));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

afterEach(() => {
  cleanup();
});

const capture = {
  items: [],
  snapshot: { task_id: "task-1", revision: 0, items: [] },
  mode: null,
  draft: null,
  candidateLabel: null,
  captureError: null,
  isRasterizing: false,
  isUploading: false,
  isMutating: false,
  mutationError: null,
  startCapture: vi.fn(),
  cancelCapture: vi.fn(),
  discardDraft: vi.fn(),
  saveDraft: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  clear: vi.fn(),
  handleIframeLoad: vi.fn(),
};

function renderPreview(onTogglePreview = vi.fn(), overrides: Record<string, unknown> = {}) {
  return {
    onTogglePreview,
    ...render(
      <TooltipProvider>
        <HtmlPreviewContent
          path="reports/index.html"
          previewUrl="http://api.test/port-proxy/session-1/43127/reports/index.html?v=4"
          taskId="task-1"
          sessionId="session-1"
          showExternalVcsLink={false}
          onTogglePreview={onTogglePreview}
          {...overrides}
        />
      </TooltipProvider>,
    ),
  };
}

describe("HtmlPreviewContent", () => {
  beforeEach(() => usePreviewCapture.mockReturnValue(capture));

  it("renders a native iframe for the published preview URL", () => {
    const { onTogglePreview } = renderPreview();

    const frame = screen.getByTestId("html-preview-frame");
    expect(frame.getAttribute("src")).toBe(
      "http://api.test/port-proxy/session-1/43127/reports/index.html?v=4",
    );
    expect(frame.getAttribute("sandbox")).toContain("allow-scripts");
    expect(frame.getAttribute("sandbox")).toContain("allow-same-origin");
    expect(screen.getByTestId("html-preview-trust-warning")).toBeTruthy();

    screen.getByRole("button", { name: "Show code" }).click();
    expect(onTogglePreview).toHaveBeenCalledOnce();
  });

  it("shows loading state while publishing", () => {
    renderPreview(vi.fn(), { previewUrl: undefined, isLoading: true });

    expect(screen.getByTestId("html-preview-loading")).toBeTruthy();
    expect(screen.queryByTestId("html-preview-frame")).toBeNull();
  });

  it("shows localized recovery copy and retries without leaving the preview surface", () => {
    const onRetry = vi.fn();
    const { onTogglePreview } = renderPreview(vi.fn(), {
      previewUrl: undefined,
      error: "session-unavailable",
      onRetry,
    });

    expect(screen.getByTestId("html-preview-error").textContent).toContain(
      "HTML preview session is not available",
    );
    screen.getByRole("button", { name: "Retry HTML preview" }).click();
    expect(onRetry).toHaveBeenCalledOnce();
    screen.getByRole("button", { name: "Show code" }).click();
    expect(onTogglePreview).toHaveBeenCalledOnce();
  });

  it("offers refresh and an explicit Browser-panel action", () => {
    const onRefresh = vi.fn();
    const onOpenInBrowser = vi.fn();
    renderPreview(vi.fn(), { onRefresh, onOpenInBrowser });

    screen.getByRole("button", { name: "Refresh HTML preview" }).click();
    expect(onRefresh).toHaveBeenCalledOnce();

    screen.getByRole("button", { name: /Open in browser panel/i }).click();
    expect(onOpenInBrowser).toHaveBeenCalledOnce();
  });

  it("uses the same task-backed annotation controls as the Browser panel", () => {
    renderPreview();

    expect(screen.getByRole("button", { name: "Annotate (0)" })).toBeTruthy();
    expect(usePreviewCapture).toHaveBeenCalledWith(
      expect.objectContaining({
        taskId: "task-1",
        enabled: true,
        source: {
          kind: "html_file",
          sessionId: "session-1",
          label: "reports/index.html",
          path: "reports/index.html",
        },
      }),
    );
  });
});
