import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import type { ComponentProps } from "react";

vi.mock("./changes-panel-file-row", () => ({
  FileRow: ({ file }: { file: { path: string } }) => (
    <li data-testid="file-row">{file.path}</li>
  ),
  BulkActionBar: () => null,
  DefaultActionButtons: () => null,
}));

vi.mock("./commit-row", () => ({
  CommitRow: () => null,
}));

vi.mock("@/hooks/use-multi-select", () => ({
  useMultiSelect: () => ({
    selectedPaths: new Set<string>(),
    isSelected: () => false,
    handleClick: vi.fn(),
    clearSelection: vi.fn(),
  }),
}));

import { FileListSection } from "./changes-panel-timeline";

afterEach(cleanup);

type Props = ComponentProps<typeof FileListSection>;

const baseProps: Omit<Props, "files" | "variant" | "isLast" | "actionLabel" | "onAction"> = {
  pendingStageFiles: new Set(),
  onOpenDiff: vi.fn(),
  onEditFile: vi.fn(),
  onStage: vi.fn(),
  onUnstage: vi.fn(),
  onDiscard: vi.fn(),
};

function file(path: string, repo?: string): Props["files"][number] {
  return {
    path,
    status: "modified",
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
    repositoryName: repo,
  };
}

describe("FileListSection — multi-repo grouping", () => {
  it("renders no repo header when files lack a repository_name (single-repo)", () => {
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[file("a.ts"), file("b.ts")]}
      />,
    );
    expect(screen.queryAllByTestId("changes-repo-header")).toHaveLength(0);
    expect(screen.getAllByTestId("file-row")).toHaveLength(2);
  });

  it("renders one header per repo when 2+ repos are present", () => {
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[
          file("src/app.tsx", "frontend"),
          file("src/api.ts", "frontend"),
          file("handlers/task.go", "backend"),
        ]}
      />,
    );
    const headers = screen.getAllByTestId("changes-repo-header");
    expect(headers).toHaveLength(2);
    expect(headers[0].textContent).toContain("frontend");
    expect(headers[0].textContent).toContain("2");
    expect(headers[1].textContent).toContain("backend");
    expect(headers[1].textContent).toContain("1");
  });

  it("shows a header for a single named repo too", () => {
    render(
      <FileListSection
        {...baseProps}
        variant="unstaged"
        isLast={false}
        actionLabel="Stage all"
        onAction={() => undefined}
        files={[file("a.ts", "only-repo")]}
      />,
    );
    expect(screen.getByTestId("changes-repo-header").textContent).toContain("only-repo");
  });
});
