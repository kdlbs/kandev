import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  ExternalVcsFileMenuItem: () => <span data-testid="external-vcs-file-menu-item" />,
  useExternalVcsFileStatus: () => ({ status: "renamed", old_path: "src/old-name.ts" }),
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => <span data-testid="file-actions-dropdown" />,
  FileActionsMenuItems: () => <span data-testid="file-actions-menu-items" />,
}));

vi.mock("@/components/editors/lsp-status-button", () => ({
  LspStatusButton: () => <span data-testid="lsp-status" />,
}));

import { MonacoEditorToolbar } from "./monaco-editor-toolbar";

afterEach(cleanup);

describe("MonacoEditorToolbar external file action", () => {
  it("uses the editor's exact repository and live file status", () => {
    render(
      <TooltipProvider>
        <MonacoEditorToolbar
          path="src/new-name.ts"
          repositoryName="frontend"
          isDirty={false}
          isSaving={false}
          diffStats={null}
          wrapEnabled={false}
          showDiffIndicators={false}
          enableComments={false}
          sessionId="session-1"
          commentCount={0}
          lspStatus={{ state: "disabled" }}
          lspProgress={{
            initializingSince: null,
            active: [],
            completed: null,
            hasReportedProgress: false,
          }}
          lspLanguage={null}
          onToggleLsp={vi.fn()}
          onToggleWrap={vi.fn()}
          onToggleDiffIndicators={vi.fn()}
          onSave={vi.fn()}
        />
      </TooltipProvider>,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: "src/new-name.ts",
      previousPath: "src/old-name.ts",
      status: "renamed",
      sessionId: "session-1",
      repositoryName: "frontend",
      size: "sm",
    });
    expect(screen.getByTestId("file-actions-dropdown")).toBeTruthy();
  });

  it("removes the toolbar LSP trigger when the active surface is the status bar", () => {
    render(
      <TooltipProvider>
        <MonacoEditorToolbar
          path="src/Main.kt"
          isDirty={false}
          isSaving={false}
          diffStats={null}
          wrapEnabled={false}
          showDiffIndicators={false}
          enableComments={false}
          sessionId="session-1"
          commentCount={0}
          lspStatus={{ state: "starting" }}
          lspProgress={{
            initializingSince: 1,
            active: [],
            completed: null,
            hasReportedProgress: false,
          }}
          lspLanguage="kotlin"
          showLspStatus={false}
          onToggleLsp={vi.fn()}
          onToggleWrap={vi.fn()}
          onToggleDiffIndicators={vi.fn()}
          onSave={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByTestId("lsp-status")).toBeNull();
  });
});

describe("MonacoEditorToolbar preview action", () => {
  const baseProps = {
    path: "reports/index.html",
    isDirty: false,
    isSaving: false,
    diffStats: null,
    wrapEnabled: false,
    showDiffIndicators: false,
    enableComments: false,
    sessionId: "session-1",
    commentCount: 0,
    lspStatus: { state: "disabled" } as const,
    lspProgress: {
      initializingSince: null,
      active: [],
      completed: null,
      hasReportedProgress: false,
    },
    lspLanguage: null,
    onToggleLsp: vi.fn(),
    onToggleWrap: vi.fn(),
    onToggleDiffIndicators: vi.fn(),
    onSave: vi.fn(),
  };

  it("labels and invokes HTML preview", () => {
    const onPreviewHtml = vi.fn();
    render(
      <TooltipProvider>
        <MonacoEditorToolbar {...baseProps} previewKind="html" onPreviewHtml={onPreviewHtml} />
      </TooltipProvider>,
    );

    screen.getByRole("button", { name: "Preview HTML" }).click();
    expect(onPreviewHtml).toHaveBeenCalledOnce();
  });
});

describe("MonacoEditorToolbar download action", () => {
  const baseProps = {
    path: "assets/report.pdf",
    isDirty: false,
    isSaving: false,
    diffStats: null,
    wrapEnabled: false,
    showDiffIndicators: false,
    enableComments: false,
    sessionId: "session-1",
    commentCount: 0,
    lspStatus: { state: "disabled" } as const,
    lspProgress: {
      initializingSince: null,
      active: [],
      completed: null,
      hasReportedProgress: false,
    },
    lspLanguage: null,
    onToggleLsp: vi.fn(),
    onToggleWrap: vi.fn(),
    onToggleDiffIndicators: vi.fn(),
    onSave: vi.fn(),
  };

  it("invokes onDownload when the download control is activated", async () => {
    const onDownload = vi.fn();
    render(
      <TooltipProvider>
        <MonacoEditorToolbar {...baseProps} onDownload={onDownload} />
      </TooltipProvider>,
    );

    const button = screen.getByRole("button", { name: "Download file" });
    button.click();
    expect(onDownload).toHaveBeenCalledTimes(1);
  });

  it("omits the download control when no handler is supplied", () => {
    render(
      <TooltipProvider>
        <MonacoEditorToolbar {...baseProps} />
      </TooltipProvider>,
    );

    expect(screen.queryByRole("button", { name: "Download file" })).toBeNull();
  });
});

describe("MonacoEditorToolbar narrow action overflow", () => {
  it("keeps dirty Save visible while exposing secondary actions through overflow", async () => {
    const originalResizeObserver = globalThis.ResizeObserver;
    const originalRect = HTMLElement.prototype.getBoundingClientRect;
    class ResizeObserverStub {
      private readonly callback: ResizeObserverCallback;

      constructor(callback: ResizeObserverCallback) {
        this.callback = callback;
      }

      observe(target: Element) {
        this.callback([{ target } as ResizeObserverEntry], this as unknown as ResizeObserver);
      }

      disconnect() {}

      unobserve() {}
    }

    globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;
    HTMLElement.prototype.getBoundingClientRect = function () {
      if (this.getAttribute("data-panel-header") === "true") {
        return {
          x: 0,
          y: 0,
          top: 0,
          left: 0,
          right: 480,
          bottom: 30,
          width: 480,
          height: 30,
          toJSON: () => ({}),
        } as DOMRect;
      }
      return originalRect.call(this);
    };

    try {
      const onSave = vi.fn();
      const onToggleWrap = vi.fn();
      render(
        <TooltipProvider>
          <MonacoEditorToolbar
            path="src/app.ts"
            isDirty={true}
            isSaving={false}
            diffStats={{ additions: 2, deletions: 1 }}
            wrapEnabled={false}
            showDiffIndicators={false}
            enableComments={false}
            sessionId="session-1"
            commentCount={0}
            lspStatus={{ state: "disabled" }}
            lspProgress={{
              initializingSince: null,
              active: [],
              completed: null,
              hasReportedProgress: false,
            }}
            lspLanguage="typescript"
            onToggleLsp={vi.fn()}
            onToggleWrap={onToggleWrap}
            onToggleDiffIndicators={vi.fn()}
            hasVcsDiff
            onSave={onSave}
            onTogglePreview={vi.fn()}
            previewKind="markdown"
            onDownload={vi.fn()}
            onDelete={vi.fn()}
          />
        </TooltipProvider>,
      );

      await waitFor(() => {
        expect(screen.getByRole("button", { name: "Show more actions" })).toBeTruthy();
      });
      expect(screen.getAllByRole("button", { name: /Save/ })).toHaveLength(1);
      expect(
        screen
          .getAllByRole("button", { name: /Save/ })
          .every((button) => !button.hasAttribute("disabled")),
      ).toBe(true);
      expect(
        document.querySelector("[data-panel-header]")?.getAttribute("data-panel-overflow"),
      ).toBe("true");
      fireEvent.pointerDown(screen.getByRole("button", { name: "Show more actions" }), {
        button: 0,
        ctrlKey: false,
      });
      await waitFor(() => {
        expect(screen.getByRole("menuitem", { name: "Enable word wrap" })).toBeTruthy();
      });
      expect(screen.getByRole("menuitem", { name: "Download file" })).toBeTruthy();
      expect(screen.getByRole("menuitem", { name: "Delete file" })).toBeTruthy();
      fireEvent.click(screen.getByRole("menuitem", { name: "Enable word wrap" }));
      expect(onToggleWrap).toHaveBeenCalledOnce();
    } finally {
      HTMLElement.prototype.getBoundingClientRect = originalRect;
      if (originalResizeObserver) {
        globalThis.ResizeObserver = originalResizeObserver;
      } else {
        // @ts-expect-error jsdom may not define ResizeObserver
        delete globalThis.ResizeObserver;
      }
    }
  });
});
