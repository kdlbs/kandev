import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { FileRow } from "./changes-panel-file-row";

const noop = () => {};
const noopSelect = () => false;

const baseFile = {
  status: "modified" as const,
  staged: false,
  plus: 18,
  minus: 3,
  oldPath: undefined,
};

function renderRow(path: string) {
  return render(
    <TooltipProvider>
      <ul>
        <FileRow
          file={{ ...baseFile, path }}
          isPending={false}
          onSelect={noopSelect}
          onOpenDiff={noop}
          onStage={noop}
          onUnstage={noop}
          onDiscard={noop}
          onEditFile={noop}
        />
      </ul>
    </TooltipProvider>,
  );
}

describe("FileRow truncation (regression: path overlaps diff stats in narrow panel)", () => {
  it("file name span allows truncation so a long name does not overflow visually", () => {
    // Bug: when the panel is narrow, a long file name renders past its container
    // and overlaps the diff counts (+/-) on the right. Fix: file name span must be
    // truncatable (truncate + min-w-0), not whitespace-nowrap+shrink-0.
    const { container } = renderRow("deeply/nested/folder/render_text_to_image_controller_test.go");
    const fileNameSpan = container.querySelector(
      "span.font-medium.text-foreground",
    ) as HTMLElement | null;

    expect(fileNameSpan).not.toBeNull();
    expect(fileNameSpan!.className).toContain("truncate");
    expect(fileNameSpan!.className).toContain("min-w-0");
    // Must NOT prevent shrinking — that's what causes the overflow.
    expect(fileNameSpan!.className).not.toContain("shrink-0");
  });

  it("folder span keeps truncate so the folder portion is shortened first", () => {
    const { container } = renderRow("a/b/c/d/file.go");
    const folderSpan = container.querySelector("span.text-foreground\\/60") as HTMLElement | null;

    expect(folderSpan).not.toBeNull();
    expect(folderSpan!.className).toContain("truncate");
  });

  it("right-side stats container has shrink-0 so it stays in place when path is long", () => {
    // Without shrink-0 on the stats column, flex layout could squeeze the
    // diff counts. Anchoring it ensures the file name truncates instead.
    const { container } = renderRow("file.go");
    // The stats container is the second direct child of the <li>
    const li = container.querySelector("li") as HTMLElement | null;
    expect(li).not.toBeNull();
    const statsContainer = li!.children[1] as HTMLElement;
    expect(statsContainer.className).toContain("shrink-0");
  });

  it("renders a file with no folder without crashing and exposes the file name", () => {
    const { container } = renderRow("README.md");
    const fileNameSpan = container.querySelector(
      "span.font-medium.text-foreground",
    ) as HTMLElement | null;
    expect(fileNameSpan?.textContent).toBe("README.md");
    // No folder element when there's no slash in the path.
    const folderSpan = container.querySelector("span.text-foreground\\/60");
    expect(folderSpan).toBeNull();
  });

  it("passes diff source + repository context on open diff", () => {
    const onOpenDiff = vi.fn();
    const { container } = render(
      <TooltipProvider>
        <ul>
          <FileRow
            file={{ ...baseFile, path: "README.md", repositoryName: "frontend" }}
            isPending={false}
            onSelect={noopSelect}
            onOpenDiff={onOpenDiff}
            onStage={noop}
            onUnstage={noop}
            onDiscard={noop}
            onEditFile={noop}
          />
        </ul>
      </TooltipProvider>,
    );

    const row = container.querySelector("[data-testid='file-row-README.md']") as HTMLElement | null;
    expect(row).not.toBeNull();
    fireEvent.click(row!);
    expect(onOpenDiff).toHaveBeenCalledWith("README.md", {
      source: "uncommitted",
      repositoryName: "frontend",
    });
  });
});
