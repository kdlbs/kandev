import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent, within } from "@testing-library/react";
import type { ReviewFile } from "./types";
import { reviewFileKey } from "./types";
import { ReviewFileTree } from "./review-file-tree";

afterEach(cleanup);

const APP_PATH = "src/app.tsx";

function file(overrides: Partial<ReviewFile>): ReviewFile {
  return {
    path: APP_PATH,
    diff: "",
    status: "modified",
    additions: 1,
    deletions: 0,
    staged: false,
    source: "uncommitted",
    ...overrides,
  };
}

type RenderOpts = Partial<Parameters<typeof ReviewFileTree>[0]>;

function renderTree(files: ReviewFile[], overrides: RenderOpts = {}) {
  const onFilterChange = vi.fn();
  const onSelectFile = vi.fn();
  const onToggleReviewed = vi.fn();
  const props = {
    files,
    reviewedFiles: new Set<string>(),
    staleFiles: new Set<string>(),
    commentCountByFile: {} as Record<string, number>,
    selectedFile: null,
    filter: "",
    onFilterChange,
    onSelectFile,
    onToggleReviewed,
    ...overrides,
  };
  render(<ReviewFileTree {...props} />);
  return { onFilterChange, onSelectFile, onToggleReviewed };
}

describe("ReviewFileTree", () => {
  it("renders one row per file", () => {
    renderTree([file({ path: "src/a.ts" }), file({ path: "src/b.ts" })]);
    expect(screen.getByText("a.ts")).toBeTruthy();
    expect(screen.getByText("b.ts")).toBeTruthy();
  });

  it("clicking a file row fires onSelectFile with the composite key", () => {
    const f = file({ path: APP_PATH, repository_name: "frontend", repository_id: "f" });
    const { onSelectFile } = renderTree([f]);
    const row = screen.getByText("app.tsx");
    fireEvent.click(row);
    expect(onSelectFile).toHaveBeenCalledWith(reviewFileKey(f));
  });

  it("toggling the reviewed checkbox fires onToggleReviewed with the composite key", () => {
    const f = file({ path: APP_PATH, repository_name: "frontend", repository_id: "f" });
    const { onToggleReviewed, onSelectFile } = renderTree([f]);
    // The checkbox is the only role=checkbox in the tree.
    const checkbox = screen.getByRole("checkbox");
    fireEvent.click(checkbox);
    expect(onToggleReviewed).toHaveBeenCalledTimes(1);
    expect(onToggleReviewed).toHaveBeenCalledWith(reviewFileKey(f), true);
    // Checkbox click must NOT also fire onSelectFile - the row's onClick is
    // stopped by the checkbox's stopPropagation handler. This invariant
    // matters because clicking "I reviewed this file" should not navigate
    // to it.
    expect(onSelectFile).not.toHaveBeenCalled();
  });

  it("reviewed checkbox renders unchecked when the file is in staleFiles", () => {
    const f = file({ path: APP_PATH, repository_name: "x", repository_id: "x" });
    const key = reviewFileKey(f);
    renderTree([f], {
      reviewedFiles: new Set([key]),
      staleFiles: new Set([key]),
    });
    // Stale takes precedence over reviewed: checkbox is unchecked.
    const checkbox = screen.getByRole("checkbox") as HTMLInputElement;
    // Radix Checkbox surfaces the state via aria-checked / data-state, not
    // the underlying input's `checked` attribute.
    expect(checkbox.getAttribute("aria-checked")).not.toBe("true");
  });

  it("reviewed checkbox renders checked when reviewed and not stale", () => {
    const f = file({ path: APP_PATH, repository_name: "x", repository_id: "x" });
    const key = reviewFileKey(f);
    renderTree([f], {
      reviewedFiles: new Set([key]),
      staleFiles: new Set<string>(),
    });
    const checkbox = screen.getByRole("checkbox");
    expect(checkbox.getAttribute("aria-checked")).toBe("true");
  });

  it("renders comment count badge for files with comments", () => {
    const f = file({ path: APP_PATH, repository_name: "x", repository_id: "x" });
    const key = reviewFileKey(f);
    renderTree([f], { commentCountByFile: { [key]: 3 } });
    expect(screen.getByText("3")).toBeTruthy();
  });

  it("filter input change fires onFilterChange", () => {
    const { onFilterChange } = renderTree([file({ path: "a.ts" })]);
    const input = screen.getByPlaceholderText("Filter changed files") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "abc" } });
    expect(onFilterChange).toHaveBeenCalledWith("abc");
  });

  it("X button on the filter input clears the filter via onFilterChange", () => {
    const { onFilterChange } = renderTree([file({ path: "a.ts" })], { filter: "abc" });
    // The X button has no accessible name; locate it via the input's sibling
    // button. With filter set, the clear button is rendered.
    const input = screen.getByPlaceholderText("Filter changed files");
    const container = input.parentElement!;
    const clear = within(container).getByRole("button");
    fireEvent.click(clear);
    expect(onFilterChange).toHaveBeenCalledWith("");
  });

  it("groups files under a repo-root header when 2+ distinct repositories are present", () => {
    renderTree([
      file({ path: "src/a.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "handlers/task.go", repository_name: "backend", repository_id: "b" }),
    ]);
    const repoHeaders = screen.getAllByTestId("repo-root-node");
    const names = repoHeaders.map((h) => h.textContent ?? "");
    expect(names.some((n) => n.includes("frontend"))).toBe(true);
    expect(names.some((n) => n.includes("backend"))).toBe(true);
  });

  it("collapsing a directory header hides its children", () => {
    renderTree([
      file({ path: "src/a.ts", repository_name: "frontend", repository_id: "f" }),
      file({ path: "handlers/task.go", repository_name: "backend", repository_id: "b" }),
    ]);
    // Both files visible by default (TreeNode expanded=true initially).
    expect(screen.getByText("a.ts")).toBeTruthy();
    expect(screen.getByText("task.go")).toBeTruthy();

    // Click the "frontend" repo root header to collapse.
    const repoHeaders = screen.getAllByTestId("repo-root-node");
    const frontend = repoHeaders.find((h) => h.textContent?.includes("frontend"))!;
    const toggle = within(frontend).getAllByRole("button")[0];
    fireEvent.click(toggle);
    // a.ts no longer rendered.
    expect(screen.queryByText("a.ts")).toBeNull();
    // backend's child is still visible.
    expect(screen.getByText("task.go")).toBeTruthy();
  });

  it("disambiguates same-named files in different repos via composite key", () => {
    const fA = file({ path: "README.md", repository_name: "frontend", repository_id: "f" });
    const fB = file({ path: "README.md", repository_name: "backend", repository_id: "b" });
    const { onSelectFile } = renderTree([fA, fB]);
    // There are two rows with the same display name; clicking each must
    // dispatch a distinct key.
    const rows = screen.getAllByText("README.md");
    expect(rows).toHaveLength(2);
    fireEvent.click(rows[0]);
    fireEvent.click(rows[1]);
    const calls = onSelectFile.mock.calls.map((c) => c[0] as string);
    expect(new Set(calls).size).toBe(2);
    expect(calls).toContain(reviewFileKey(fA));
    expect(calls).toContain(reviewFileKey(fB));
  });
});
