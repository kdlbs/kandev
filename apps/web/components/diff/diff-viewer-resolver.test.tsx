import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/hooks/use-editor-resolver", () => ({
  useEditorProvider: () => "monaco",
}));

vi.mock("@/components/editors/monaco/monaco-diff-viewer", () => ({
  MonacoDiffViewer: (props: Record<string, unknown>) => (
    <span data-testid="monaco-diff-props" data-props={JSON.stringify(props)} />
  ),
}));

vi.mock("./diff-viewer", () => ({
  DiffViewer: () => null,
  DiffViewInline: () => null,
}));

import { DiffViewerResolved } from "./diff-viewer-resolver";

afterEach(cleanup);

describe("DiffViewerResolved Monaco context", () => {
  it("preserves external-link context while stripping Pierre-only props", () => {
    render(
      <DiffViewerResolved
        data={{
          filePath: "src/app.ts",
          oldContent: "old",
          newContent: "new",
          additions: 1,
          deletions: 1,
        }}
        enableComments
        enableExpansion
        baseRef="HEAD"
        repo="frontend"
        taskId="task-1"
        sessionId="session-1"
        repositoryId="repo-1"
        status="modified"
        previousPath="src/old.ts"
        publishedBranch="feature/external-links"
        externalBaseBranch="main"
      />,
    );

    const props = JSON.parse(screen.getByTestId("monaco-diff-props").dataset.props ?? "{}");
    expect(props).toMatchObject({
      repo: "frontend",
      taskId: "task-1",
      sessionId: "session-1",
      repositoryId: "repo-1",
      status: "modified",
      previousPath: "src/old.ts",
      publishedBranch: "feature/external-links",
      externalBaseBranch: "main",
    });
    expect(props).not.toHaveProperty("enableComments");
    expect(props).not.toHaveProperty("enableExpansion");
    expect(props).not.toHaveProperty("baseRef");
  });
});
