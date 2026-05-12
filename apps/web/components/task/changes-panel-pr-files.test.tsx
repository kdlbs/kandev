import { describe, it, expect, vi } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import { PRFilesGroupedList } from "./changes-panel-pr-files";

describe("PRFilesGroupedList", () => {
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
});
