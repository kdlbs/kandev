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

describe("FileRow hover swap (stats <-> actions occupy same cell)", () => {
  // Goal: hovering must not shift the file name. The stats group and the
  // hover-actions group live in the same CSS grid cell — stats fade out and
  // actions fade in, so the right cluster's width is stable.
  it("stats container fades out on hover and never intercepts pointer events", () => {
    const { container } = renderRow("file.go");
    const li = container.querySelector("li") as HTMLElement;
    const gridWrapper = li.children[1] as HTMLElement;
    const statsLayer = gridWrapper.children[0] as HTMLElement;
    expect(statsLayer.className).toContain("group-hover:opacity-0");
    expect(statsLayer.className).toContain("transition-opacity");
    // Defensive: stats has no interactive children today, but blocking pointer
    // events on this always-non-interactive layer prevents a future regression
    // if someone adds a clickable element here.
    expect(statsLayer.className).toContain("pointer-events-none");
  });

  it("actions container fades in on hover and only accepts pointer/keyboard events while hovered", () => {
    const { container } = renderRow("file.go");
    const li = container.querySelector("li") as HTMLElement;
    const gridWrapper = li.children[1] as HTMLElement;
    const actionsLayer = gridWrapper.children[1] as HTMLElement;
    expect(actionsLayer.className).toContain("opacity-0");
    expect(actionsLayer.className).toContain("group-hover:opacity-100");
    expect(actionsLayer.className).toContain("transition-opacity");
    // Critical: the actions div is the top child in the same grid cell, so
    // without pointer-events gating it would eat clicks aimed at the
    // visually-shown stats. (Keyboard Tab focus is a separate concern — a
    // pre-existing gap not closed by `pointer-events: none`, which only
    // blocks mouse/touch.)
    expect(actionsLayer.className).toContain("pointer-events-none");
    expect(actionsLayer.className).toContain("group-hover:pointer-events-auto");
  });

  it("wrapper stacks both layers into one grid cell so width is max(stats, actions)", () => {
    const { container } = renderRow("file.go");
    const li = container.querySelector("li") as HTMLElement;
    const gridWrapper = li.children[1] as HTMLElement;
    expect(gridWrapper.className).toContain("grid");
    // Arbitrary child selector pins every direct child to row 1 / col 1 so
    // they overlap instead of laying out side-by-side.
    expect(gridWrapper.className).toContain("col-start-1");
    expect(gridWrapper.className).toContain("row-start-1");
  });
});

describe("FileRow tree-mode hover stage action", () => {
  it("keeps the stage button in the icon slot instead of the right hover actions", () => {
    const { container } = render(
      <TooltipProvider>
        <ul>
          <FileRow
            file={{ ...baseFile, path: "src/file.go" }}
            isPending={false}
            treeMode
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

    const row = container.querySelector("[data-changes-file='src/file.go']") as HTMLElement | null;
    expect(row).not.toBeNull();

    const iconSlot = row!.querySelector("[data-testid='file-row-icon-action-slot']");
    expect(iconSlot).not.toBeNull();
    expect(iconSlot!.classList.contains("size-4")).toBe(true);
    expect(iconSlot!.querySelector("button[title='Stage file']")).not.toBeNull();

    const rightActions = row!.querySelector("[data-testid='file-row-hover-actions']");
    expect(rightActions).not.toBeNull();
    expect(rightActions!.querySelector("button[title='Stage file']")).toBeNull();
  });

  it("keeps the unstage button in the same icon slot for staged files", () => {
    const { container } = render(
      <TooltipProvider>
        <ul>
          <FileRow
            file={{ ...baseFile, path: "src/staged.go", staged: true }}
            isPending={false}
            treeMode
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

    const row = container.querySelector(
      "[data-changes-file='src/staged.go']",
    ) as HTMLElement | null;
    expect(row).not.toBeNull();

    const iconSlot = row!.querySelector("[data-testid='file-row-icon-action-slot']");
    expect(iconSlot).not.toBeNull();
    expect(iconSlot!.classList.contains("size-4")).toBe(true);
    expect(iconSlot!.querySelector("button[title='Unstage file']")).not.toBeNull();

    const rightActions = row!.querySelector("[data-testid='file-row-hover-actions']");
    expect(rightActions).not.toBeNull();
    expect(rightActions!.querySelector("button[title='Unstage file']")).toBeNull();
  });
});

describe("FileRow active-tab highlight", () => {
  it("renders data-active='true' and bg-accent/60 when isActive", () => {
    const { container } = render(
      <TooltipProvider>
        <ul>
          <FileRow
            file={{ ...baseFile, path: "active.ts" }}
            isPending={false}
            isActive
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

    const row = container.querySelector("[data-changes-file='active.ts']") as HTMLElement | null;
    expect(row).not.toBeNull();
    expect(row!.getAttribute("data-active")).toBe("true");
    expect(row!.className).toContain("bg-accent/60");
  });

  it("keeps highlight when isActive and isSelected are both true", () => {
    const { container } = render(
      <TooltipProvider>
        <ul>
          <FileRow
            file={{ ...baseFile, path: "both.ts" }}
            isPending={false}
            isActive
            isSelected
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

    const row = container.querySelector("[data-changes-file='both.ts']") as HTMLElement | null;
    expect(row).not.toBeNull();
    expect(row!.getAttribute("data-active")).toBe("true");
    expect(row!.getAttribute("data-selected")).toBe("true");
    expect(row!.className).toContain("bg-accent/60");
  });
});
