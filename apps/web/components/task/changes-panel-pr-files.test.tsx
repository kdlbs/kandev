import { describe, it, expect, vi, afterEach } from "vitest";
import { render, fireEvent, screen, cleanup } from "@testing-library/react";
import { PRFilesGroupedList } from "./changes-panel-pr-files";

describe("PRFilesGroupedList", () => {
  afterEach(() => cleanup());

  it("passes diff source + repository context on open diff", () => {
    const onOpenDiff = vi.fn();
    render(
      <PRFilesGroupedList
        files={[
          {
            path: "README.md",
            status: "modified",
            plus: 1,
            minus: 0,
            oldPath: undefined,
            repository_name: "backend",
          },
        ]}
        onOpenDiff={onOpenDiff}
      />,
    );

    fireEvent.click(screen.getByText("README.md"));
    expect(onOpenDiff).toHaveBeenCalledWith("README.md", {
      source: "pr",
      repositoryName: "backend",
    });
  });

  it("does not pass an empty single-repo stamp as repository context", () => {
    const onOpenDiff = vi.fn();
    render(
      <PRFilesGroupedList
        files={[
          {
            path: "README.md",
            status: "modified",
            plus: 1,
            minus: 0,
            oldPath: undefined,
            repository_name: "",
          },
        ]}
        onOpenDiff={onOpenDiff}
      />,
    );

    fireEvent.click(screen.getByText("README.md"));
    expect(onOpenDiff).toHaveBeenCalledWith("README.md", {
      source: "pr",
      repositoryName: undefined,
    });
  });

  it("shows previous-path context for a moved PR file", () => {
    render(
      <PRFilesGroupedList
        files={[
          {
            path: "src/new-name.ts",
            status: "renamed",
            plus: 0,
            minus: 0,
            oldPath: "src/old-name.ts",
            repository_name: "",
          },
        ]}
        onOpenDiff={vi.fn()}
      />,
    );

    expect(screen.getByRole("img", { name: "Moved from src/old-name.ts" })).toBeTruthy();
  });
});
