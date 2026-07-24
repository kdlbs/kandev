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
  ExternalVcsFileLink: (props: Record<string, unknown>) =>
    props.sessionId ? (
      <span data-testid="external-vcs-file-link-props" data-props={JSON.stringify(props)} />
    ) : null,
  useExternalVcsFileStatus: () => ({ status: "renamed", old_path: "docs/old.md" }),
}));

import { CodeMirrorCodeEditor } from "./codemirror-code-editor";

afterEach(cleanup);

const baseProps = {
  path: "docs/new.md",
  content: "# New",
  originalContent: "# Old",
  isDirty: false,
  isSaving: false,
  onChange: vi.fn(),
  onSave: vi.fn(),
};

describe("CodeMirrorCodeEditor external file action", () => {
  it("forwards the editor's repository and live rename context", () => {
    render(
      <TooltipProvider>
        <CodeMirrorCodeEditor
          {...baseProps}
          sessionId="session-1"
          taskId="task-1"
          repositoryId="repo-1"
          repo="frontend"
        />
      </TooltipProvider>,
    );

    const props = JSON.parse(
      screen.getByTestId("external-vcs-file-link-props").dataset.props ?? "{}",
    );
    expect(props).toEqual({
      filePath: "docs/new.md",
      previousPath: "docs/old.md",
      status: "renamed",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryName: "frontend",
      size: "sm",
    });
  });

  it("omits the action without external repository context", () => {
    render(
      <TooltipProvider>
        <CodeMirrorCodeEditor {...baseProps} />
      </TooltipProvider>,
    );

    expect(screen.queryByTestId("external-vcs-file-link-props")).toBeNull();
  });
});
