import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { ExportFileTree } from "./export-file-tree";
import type { ExportFile } from "./export-types";
import { buildFileTree } from "./export-utils";

afterEach(cleanup);

const ALPHA_PATH = "agents/alpha.yml";
const BETA_PATH = "agents/beta.yml";
const FOO_PATH = "skills/foo.yml";
const ROOT_PATH = "kandev.yml";

const FILES: ExportFile[] = [
  { path: ROOT_PATH, content: "name: root" },
  { path: ALPHA_PATH, content: "name: alpha" },
  { path: BETA_PATH, content: "name: beta" },
  { path: FOO_PATH, content: "name: foo" },
];

const ALPHA_NAME = "alpha.yml";
const CHECKBOX_SELECTOR = "[role='checkbox']";

type RenderOpts = Partial<Parameters<typeof ExportFileTree>[0]>;

function renderExport(overrides: RenderOpts = {}) {
  const tree = buildFileTree(FILES);
  const onSelectedPathsChange = vi.fn();
  const onPreviewPathChange = vi.fn();
  render(
    <ExportFileTree
      tree={tree}
      selectedPaths={new Set(FILES.map((f) => f.path))}
      onSelectedPathsChange={onSelectedPathsChange}
      previewPath={null}
      onPreviewPathChange={onPreviewPathChange}
      {...overrides}
    />,
  );
  return { onSelectedPathsChange, onPreviewPathChange };
}

describe("ExportFileTree", () => {
  it("renders all directories expanded by default", () => {
    renderExport();
    // All dirs render expanded on mount, so every leaf file is visible.
    expect(screen.getByText(ALPHA_NAME)).toBeTruthy();
    expect(screen.getByText("beta.yml")).toBeTruthy();
    expect(screen.getByText("foo.yml")).toBeTruthy();
    expect(screen.getByText(ROOT_PATH)).toBeTruthy();
  });

  it("clicking a file row fires onPreviewPathChange with its path", () => {
    const { onPreviewPathChange } = renderExport();
    fireEvent.click(screen.getByText(ALPHA_NAME));
    expect(onPreviewPathChange).toHaveBeenCalledWith(ALPHA_PATH);
  });

  it("clicking a directory row collapses it and hides children", () => {
    renderExport();
    fireEvent.click(screen.getByText("agents"));
    expect(screen.queryByText(ALPHA_NAME)).toBeNull();
    expect(screen.queryByText("beta.yml")).toBeNull();
    // Sibling dir's children remain visible.
    expect(screen.getByText("foo.yml")).toBeTruthy();
  });

  it("typing into the search input hides non-matching rows", () => {
    renderExport();
    const input = screen.getByPlaceholderText("Search files...");
    fireEvent.change(input, { target: { value: "alpha" } });
    expect(screen.getByText(ALPHA_NAME)).toBeTruthy();
    expect(screen.queryByText("beta.yml")).toBeNull();
    expect(screen.queryByText("foo.yml")).toBeNull();
    expect(screen.queryByText(ROOT_PATH)).toBeNull();
  });

  it("search keeps an ancestor visible when a descendant matches", () => {
    renderExport();
    const input = screen.getByPlaceholderText("Search files...");
    fireEvent.change(input, { target: { value: "alpha" } });
    // The "agents" dir is shown because alpha.yml lives under it.
    expect(screen.getByText("agents")).toBeTruthy();
    // The unrelated "skills" dir is hidden because none of its descendants match.
    expect(screen.queryByText("skills")).toBeNull();
  });

  it("toggling an unchecked file checkbox adds its path via onSelectedPathsChange", () => {
    const { onSelectedPathsChange } = renderExport({
      selectedPaths: new Set<string>(),
    });
    // alpha.yml row's checkbox is rendered alongside the row.
    const row = screen.getByText(ALPHA_NAME).parentElement!;
    const box = row.querySelector(CHECKBOX_SELECTOR) as HTMLElement;
    fireEvent.click(box);
    expect(onSelectedPathsChange).toHaveBeenCalledTimes(1);
    const next = onSelectedPathsChange.mock.calls[0][0] as Set<string>;
    expect(next.has(ALPHA_PATH)).toBe(true);
  });

  it("toggling a directory checkbox propagates to every descendant", () => {
    const { onSelectedPathsChange } = renderExport({
      selectedPaths: new Set<string>(),
    });
    const row = screen.getByText("agents").parentElement!;
    const box = row.querySelector(CHECKBOX_SELECTOR) as HTMLElement;
    fireEvent.click(box);
    const next = onSelectedPathsChange.mock.calls[0][0] as Set<string>;
    expect(next.has(ALPHA_PATH)).toBe(true);
    expect(next.has(BETA_PATH)).toBe(true);
    // Unrelated branch is untouched.
    expect(next.has(FOO_PATH)).toBe(false);
  });

  it("dir checkbox renders indeterminate when only some descendants are selected", () => {
    renderExport({ selectedPaths: new Set<string>([ALPHA_PATH]) });
    const row = screen.getByText("agents").parentElement!;
    const box = row.querySelector(CHECKBOX_SELECTOR) as HTMLElement;
    expect(box.getAttribute("data-state")).toBe("indeterminate");
  });

  it("dir checkbox renders fully checked when every descendant is selected", () => {
    renderExport({
      selectedPaths: new Set<string>([ALPHA_PATH, BETA_PATH]),
    });
    const row = screen.getByText("agents").parentElement!;
    const box = row.querySelector(CHECKBOX_SELECTOR) as HTMLElement;
    expect(box.getAttribute("data-state")).toBe("checked");
  });

  it("checkbox click does not bubble to the row's preview handler", () => {
    const { onPreviewPathChange } = renderExport({
      selectedPaths: new Set<string>(),
    });
    const row = screen.getByText(ALPHA_NAME).parentElement!;
    const box = row.querySelector(CHECKBOX_SELECTOR) as HTMLElement;
    fireEvent.click(box);
    expect(onPreviewPathChange).not.toHaveBeenCalled();
  });

  it("previewPath highlights the matching file row", () => {
    renderExport({ previewPath: ALPHA_PATH });
    const row = screen.getByText(ALPHA_NAME).parentElement!;
    expect(row.className).toContain("bg-accent");
  });
});
