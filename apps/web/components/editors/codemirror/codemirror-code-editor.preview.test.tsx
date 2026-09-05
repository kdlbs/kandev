import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@uiw/react-codemirror", () => ({
  default: () => <div data-testid="codemirror" />,
}));

vi.mock("./use-codemirror-editor-state", () => ({
  useCodeMirrorEditorState: () => ({
    comments: [],
    diffStats: null,
    extensions: [],
    floatingButtonPos: null,
    textSelection: null,
    commentView: null,
    wrapEnabled: false,
    setWrapEnabled: vi.fn(),
    handleChange: vi.fn(),
  }),
}));

vi.mock("./use-codemirror-walkthrough-range", () => ({
  useCodeMirrorWalkthroughRange: () => null,
}));

vi.mock("@/components/editors/external-vcs-file-link", () => ({
  ExternalVcsFileLink: () => null,
  useExternalVcsFileStatus: () => null,
}));

import { CodeMirrorCodeEditor } from "./codemirror-code-editor";

afterEach(cleanup);

describe("CodeMirrorCodeEditor preview action", () => {
  it("labels and invokes HTML preview", () => {
    const onPreviewHtml = vi.fn();
    render(
      <TooltipProvider>
        <CodeMirrorCodeEditor
          path="reports/index.html"
          content="<h1>Report</h1>"
          originalContent=""
          isDirty={false}
          isSaving={false}
          previewKind="html"
          onPreviewHtml={onPreviewHtml}
          onChange={vi.fn()}
          onSave={vi.fn()}
        />
      </TooltipProvider>,
    );

    screen.getByRole("button", { name: "Preview HTML" }).click();
    expect(onPreviewHtml).toHaveBeenCalledOnce();
  });
});
