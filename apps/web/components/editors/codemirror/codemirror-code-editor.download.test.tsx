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

const baseProps = {
  path: "assets/report.pdf",
  content: "",
  originalContent: "",
  isDirty: false,
  isSaving: false,
  onChange: vi.fn(),
  onSave: vi.fn(),
};

describe("CodeMirrorCodeEditor download action", () => {
  it("invokes onDownload when the download control is activated", () => {
    const onDownload = vi.fn();
    render(
      <TooltipProvider>
        <CodeMirrorCodeEditor {...baseProps} onDownload={onDownload} />
      </TooltipProvider>,
    );

    screen.getByRole("button", { name: "Download file" }).click();
    expect(onDownload).toHaveBeenCalledTimes(1);
  });

  it("omits the download control when no handler is supplied", () => {
    render(
      <TooltipProvider>
        <CodeMirrorCodeEditor {...baseProps} />
      </TooltipProvider>,
    );

    expect(screen.queryByRole("button", { name: "Download file" })).toBeNull();
  });
});
