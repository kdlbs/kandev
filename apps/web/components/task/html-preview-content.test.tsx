import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PreviewRuntimeError } from "@/lib/html-preview/preview-runtime-types";
import type { PreviewEvent, PreviewSnapshot } from "@/lib/html-preview/preview-runtime-types";
import { HtmlPreviewContent } from "./html-preview-content";

const mocks = vi.hoisted(() => ({
  createPreviewRuntime: vi.fn(),
  runtime: {
    load: vi.fn(),
    dispatch: vi.fn(),
    dispose: vi.fn(),
  },
}));

vi.mock("@/lib/html-preview/preview-runtime", () => ({
  createPreviewRuntime: mocks.createPreviewRuntime,
}));

vi.mock("@/lib/html-preview/preview-surface", () => ({
  PreviewSurface: ({
    snapshot,
    onEvent,
  }: {
    snapshot: PreviewSnapshot;
    onEvent: (event: PreviewEvent) => void;
  }) => (
    <div
      data-testid="html-preview-surface"
      onClick={() => onEvent({ type: "click", nodeId: "button" })}
    >
      {snapshot.root.text}
    </div>
  ),
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

const snapshot: PreviewSnapshot = {
  protocolVersion: 1,
  root: {
    id: "body",
    tagName: "body",
    attributes: {},
    styles: {},
    text: "Runtime output",
    children: [],
    eventTypes: [],
  },
  resources: [],
  diagnostics: [],
};

afterEach(() => {
  cleanup();
  mocks.createPreviewRuntime.mockReset();
  mocks.runtime.load.mockReset();
  mocks.runtime.dispatch.mockReset();
  mocks.runtime.dispose.mockReset();
});

function renderPreview(onTogglePreview = vi.fn(), fail = false) {
  mocks.createPreviewRuntime.mockReturnValue(mocks.runtime);
  if (fail) mocks.runtime.load.mockRejectedValue(new PreviewRuntimeError("runtime-error"));
  else mocks.runtime.load.mockResolvedValue(snapshot);
  return {
    onTogglePreview,
    ...render(
      <TooltipProvider>
        <HtmlPreviewContent
          path="reports/index.html"
          content="<button>Run</button><script>document.body.textContent = 'Runtime output'</script>"
          showExternalVcsLink={false}
          onTogglePreview={onTogglePreview}
        />
      </TooltipProvider>,
    ),
  };
}

describe("HtmlPreviewContent", () => {
  it("loads the current buffer into the runtime surface and keeps the source toggle", async () => {
    const { onTogglePreview } = renderPreview();

    expect(screen.getByTestId("html-preview-loading")).not.toBeNull();
    await waitFor(() => expect(screen.getByTestId("html-preview-surface")).not.toBeNull());
    expect(mocks.runtime.load).toHaveBeenCalledWith(
      expect.stringContaining("<button>Run</button>"),
    );
    expect(screen.queryByTestId("html-preview-frame")).toBeNull();

    screen.getByRole("button", { name: "Show code" }).click();
    expect(onTogglePreview).toHaveBeenCalledOnce();
  });

  it("shows a localized recovery message when the runtime fails", async () => {
    const { onTogglePreview } = renderPreview(undefined, true);

    await waitFor(() => expect(screen.getByTestId("html-preview-error")).not.toBeNull());
    expect(screen.getByTestId("html-preview-error").textContent).toContain(
      "HTML preview stopped safely",
    );
    screen.getByRole("button", { name: "Show code" }).click();
    expect(onTogglePreview).toHaveBeenCalledOnce();
  });
});
