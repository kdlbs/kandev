import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: (props: Record<string, unknown>) => (
    <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
  ),
  useExternalVcsFileStatus: () => ({ status: "renamed", old_path: "src/old-name.ts" }),
}));

vi.mock("@/components/editors/file-actions-dropdown", () => ({
  FileActionsDropdown: () => <span data-testid="file-actions-dropdown" />,
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
});
