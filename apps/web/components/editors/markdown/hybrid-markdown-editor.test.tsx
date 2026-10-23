import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { SourcePositionHandle } from "../source-position";
import { installTableGeometry, upstream } from "./hybrid-markdown-editor.test-helpers";

vi.mock(
  "@vscode/markdown-editor",
  async () => (await import("./hybrid-markdown-editor.test-helpers")).upstream,
);
vi.mock("@vscode/markdown-editor/editor.css", () => ({}));
vi.mock("@vscode/markdown-editor/themes/default.css", () => ({}));

import { HybridMarkdownEditor } from "./hybrid-markdown-editor";

const TABLE_ROW_INSERT_0 = "markdown-table-row-insert-0";
const TABLE_ROW_INSERT_1 = "markdown-table-row-insert-1";
const TABLE_COLUMN_INSERT_0 = "markdown-table-column-insert-0";
const TABLE_COLUMN_INSERT_1 = "markdown-table-column-insert-1";
const TABLE_RESIZER_0 = "markdown-table-resizer-0";

afterEach(cleanup);

beforeEach(() => {
  vi.stubGlobal("EditContext", class EditContext {});
  upstream.state.models.length = 0;
  upstream.state.views.length = 0;
  upstream.state.controllers.length = 0;
  upstream.state.histories.length = 0;
  upstream.state.syntaxHighlighters.length = 0;
  upstream.state.failView = false;
});

describe("HybridMarkdownEditor lifecycle", () => {
  it("mounts one source-preserving model, view, controller, and local history", async () => {
    const source = "# Keep this source\n\n<div>Unsupported</div>\n";
    const onChange = vi.fn();
    const { unmount } = render(
      <HybridMarkdownEditor content={source} readOnly={false} onChange={onChange} />,
    );

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    const controller = upstream.state.controllers[0];
    expect(model.sourceText.set).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ value: source }),
      undefined,
      undefined,
    );
    expect(model.readonlyMode.set).toHaveBeenNthCalledWith(1, false, undefined, undefined);
    expect(controller.options).toEqual(
      expect.objectContaining({ historyStrategy: expect.anything() }),
    );
    expect((view.options as { classNames: string[] }).classNames).toEqual(
      expect.arrayContaining(["kandev-hybrid-markdown-editor"]),
    );
    expect((view.options as { syntaxHighlighter: unknown }).syntaxHighlighter).toBe(
      upstream.state.syntaxHighlighters[0],
    );
    expect(view.element.parentElement).toBeTruthy();
    expect(screen.getByTestId("hybrid-markdown-editor").className).toContain(
      "kandev-hybrid-markdown-editor-root",
    );

    model.applyUserEdit({
      replacements: [
        {
          replaceRange: { start: 2, endExclusive: 6 },
          newText: "Edited",
        },
      ],
    });
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith("# Edited this source\n\n<div>Unsupported</div>\n"),
    );

    unmount();
    expect(model.dispose).toHaveBeenCalledOnce();
    expect(view.dispose).toHaveBeenCalledOnce();
    expect(controller.dispose).toHaveBeenCalledOnce();
    expect(controller.dispose.mock.invocationCallOrder[0]).toBeLessThan(
      view.dispose.mock.invocationCallOrder[0],
    );
    expect(model.sourceEditSubscription.dispose).toHaveBeenCalledOnce();
    expect(upstream.state.syntaxHighlighters[0].dispose).toHaveBeenCalledOnce();
  });

  it("notifies the host after the editor applies an inline source edit", async () => {
    const onChange = vi.fn();
    render(
      <HybridMarkdownEditor content="Before opening a PR" readOnly={false} onChange={onChange} />,
    );
    const model = upstream.state.models[0];

    model.applyUserEdit({
      replacements: [
        {
          replaceRange: { start: 6, endExclusive: 6 },
          newText: "!",
        },
      ],
    });

    expect(onChange).not.toHaveBeenCalled();
    await waitFor(() => expect(onChange).toHaveBeenCalledWith("Before! opening a PR"));
  });

  it("replaces the model source for clean host updates without echoing an edit", () => {
    const { rerender } = render(
      <HybridMarkdownEditor content="# Before" readOnly={false} onChange={vi.fn()} />,
    );
    const model = upstream.state.models[0];

    rerender(<HybridMarkdownEditor content="# After" readOnly={false} onChange={vi.fn()} />);

    expect(model.replaceSourceText).toHaveBeenCalledWith(
      expect.objectContaining({ value: "# After" }),
    );
  });
});

describe("HybridMarkdownEditor contracts", () => {
  it("passes link, baseline, gutter, and comment contracts through the adapter", () => {
    const onOpenLink = vi.fn();
    const onComment = vi.fn();
    render(
      <HybridMarkdownEditor
        content="# Heading"
        readOnly
        baseline="# Original"
        gutterMarkers={[{ start: 0, endExclusive: 2, kind: "modified" }]}
        comments={[{ id: "comment-1", start: 0, endExclusive: 2, body: "Review" }]}
        onOpenLink={onOpenLink}
        onComment={onComment}
        onChange={vi.fn()}
      />,
    );

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    const options = view.options as {
      onOpenLink: (url: string, event: MouseEvent) => false | void;
    };
    expect(options.onOpenLink("https://example.com", new MouseEvent("click"))).toBeUndefined();

    expect(onOpenLink).toHaveBeenCalledWith("https://example.com");
    expect(model.baseline.set).toHaveBeenLastCalledWith(
      expect.objectContaining({ value: "# Original" }),
      undefined,
      undefined,
    );
    expect(model.gutterMarkers.set).toHaveBeenLastCalledWith(
      [expect.objectContaining({ type: "modified" })],
      undefined,
      undefined,
    );
    expect(onComment).not.toHaveBeenCalled();
  });

  it("returns false for an unhandled link so same-document anchors stay native", () => {
    const onOpenLink = vi.fn().mockReturnValue(false);
    render(<HybridMarkdownEditor content="# Heading" onChange={vi.fn()} onOpenLink={onOpenLink} />);

    const view = upstream.state.views[0];
    const options = view.options as {
      onOpenLink: (url: string, event: MouseEvent) => false | void;
    };

    expect(options.onOpenLink("#heading", new MouseEvent("click"))).toBe(false);
    expect(onOpenLink).toHaveBeenCalledWith("#heading");
  });

  it("reports initialization failures and requests the Source fallback", () => {
    const diagnostic = vi.spyOn(console, "warn").mockImplementation(() => {});
    upstream.state.failView = true;
    const onError = vi.fn();
    const onSourceFallback = vi.fn();

    render(
      <HybridMarkdownEditor
        content="# Safe source"
        onChange={vi.fn()}
        onError={onError}
        onSourceFallback={onSourceFallback}
      />,
    );

    expect(diagnostic.mock.calls).toEqual([
      [
        "hybrid_markdown_editor_failure",
        {
          fileType: "markdown",
          mode: "edit",
          operation: "initialize",
          upstreamVersion: "0.0.2-84",
        },
      ],
    ]);
    diagnostic.mockRestore();
    expect(onError).toHaveBeenCalledWith(expect.objectContaining({ message: "view failed" }));
    expect(onSourceFallback).toHaveBeenCalledOnce();
    expect(upstream.state.models[0].dispose).toHaveBeenCalledOnce();
  });
});

describe("HybridMarkdownEditor table edge insertion contracts", () => {
  it("renders row and column actions in edge gutters and maps them to visible indices", async () => {
    const source = "| A | B |\n| --- | --- |\n| 1 | 2 |\n| 3 | 4 |\n";
    const onChange = vi.fn();
    render(<HybridMarkdownEditor content={source} onChange={onChange} />);

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    upstream.setupActiveTable(
      model,
      view,
      source,
      "<tr><td>1</td><td>2</td></tr><tr><td>3</td><td>4</td></tr>",
    );

    const table = view.element.querySelector("table") as HTMLTableElement;
    installTableGeometry(table);

    await waitFor(() => expect(screen.queryByTestId(TABLE_ROW_INSERT_0)).not.toBeNull());
    expect(screen.queryByTestId(TABLE_ROW_INSERT_1)).not.toBeNull();
    expect(screen.queryByTestId(TABLE_COLUMN_INSERT_0)).not.toBeNull();
    expect(screen.queryByTestId(TABLE_COLUMN_INSERT_1)).not.toBeNull();
    expect(screen.getByTestId(TABLE_ROW_INSERT_0).closest("table")).toBeNull();

    fireEvent.click(screen.getByTestId(TABLE_ROW_INSERT_0));

    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith(
        "| A | B |\n| --- | --- |\n|  |  |\n| 1 | 2 |\n| 3 | 4 |\n",
      ),
    );
  });

  it("maps a column edge action to one positional source edit", async () => {
    const source = "| A | B |\n| --- | --- |\n| 1 | 2 |\n";
    const onChange = vi.fn();
    render(<HybridMarkdownEditor content={source} onChange={onChange} />);

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    upstream.setupActiveTable(model, view, source, "<tr><td>1</td><td>2</td></tr>");
    const table = view.element.querySelector("table") as HTMLTableElement;
    installTableGeometry(table);

    await waitFor(() => expect(screen.queryByTestId(TABLE_COLUMN_INSERT_0)).not.toBeNull());
    fireEvent.click(screen.getByTestId(TABLE_COLUMN_INSERT_0));

    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith(
        "| A |  | B |\n| --- | --- | --- |\n| 1 |  | 2 |\n",
      ),
    );
    expect(upstream.state.histories[0].record).toHaveBeenCalledOnce();
  });
});

describe("HybridMarkdownEditor table edge resize contracts", () => {
  it("resizes a table boundary with the keyboard without changing Markdown source", async () => {
    const source = "| A | B |\n| --- | --- |\n| 1 | 2 |\n";
    const onChange = vi.fn();
    render(<HybridMarkdownEditor content={source} onChange={onChange} />);

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    upstream.setupActiveTable(model, view, source, "<tr><td>1</td><td>2</td></tr>");

    const table = view.element.querySelector("table") as HTMLTableElement;
    installTableGeometry(table);

    await waitFor(() => expect(screen.queryByTestId(TABLE_RESIZER_0)).not.toBeNull());
    const resizer = screen.getByTestId(TABLE_RESIZER_0);
    expect(resizer.getAttribute("role")).toBe("separator");
    expect(
      Number.parseFloat(resizer.getAttribute("style")?.match(/top:\s*([^;]+)/)?.[1] ?? "0"),
    ).toBeLessThan(0);
    expect(
      Number.parseFloat(resizer.getAttribute("style")?.match(/height:\s*([^;]+)/)?.[1] ?? "0"),
    ).toBeLessThan(table.getBoundingClientRect().height);
    fireEvent.keyDown(resizer, { key: "ArrowRight" });

    expect(onChange).not.toHaveBeenCalled();
    expect(table.querySelector("colgroup")).not.toBeNull();
    expect(table.querySelector("col")?.getAttribute("style")).toContain("128px");

    table.querySelector("colgroup")?.remove();
    await waitFor(() =>
      expect(table.querySelector("col")?.getAttribute("style")).toContain("128px"),
    );
    expect(onChange).not.toHaveBeenCalled();
  });

  it("keeps row and column insertion available for one-column tables", async () => {
    const source = "| A |\n| --- |\n| 1 |\n";
    const onChange = vi.fn();
    render(<HybridMarkdownEditor content={source} onChange={onChange} />);

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    upstream.setupActiveTable(model, view, source, "<tr><td>1</td></tr>", 1);
    const table = view.element.querySelector("table") as HTMLTableElement;
    installTableGeometry(table);

    await waitFor(() => expect(screen.queryByTestId(TABLE_ROW_INSERT_0)).not.toBeNull());
    expect(screen.queryByTestId(TABLE_ROW_INSERT_1)).not.toBeNull();
    expect(screen.queryByTestId(TABLE_COLUMN_INSERT_0)).not.toBeNull();
    expect(screen.queryByTestId(TABLE_RESIZER_0)).toBeNull();

    fireEvent.click(screen.getByTestId(TABLE_COLUMN_INSERT_0));

    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith("| A |  |\n| --- | --- |\n| 1 |  |\n"),
    );
  });
});

describe("HybridMarkdownEditor table edge pointer lifecycle", () => {
  it("restores a drag when the browser reports lost pointer capture", async () => {
    const source = "| A | B |\n| --- | --- |\n| 1 | 2 |\n";
    const onChange = vi.fn();
    render(<HybridMarkdownEditor content={source} onChange={onChange} />);

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    upstream.setupActiveTable(model, view, source, "<tr><td>1</td><td>2</td></tr>");
    const table = view.element.querySelector("table") as HTMLTableElement;
    installTableGeometry(table);

    await waitFor(() => expect(screen.queryByTestId(TABLE_RESIZER_0)).not.toBeNull());
    const resizer = screen.getByTestId(TABLE_RESIZER_0);
    fireEvent.pointerDown(resizer, {
      button: 0,
      clientX: 168,
      pointerId: 7,
      pointerType: "touch",
    });
    fireEvent.pointerMove(resizer, {
      clientX: 184,
      pointerId: 7,
      pointerType: "touch",
    });
    expect(table.querySelector("col")?.getAttribute("style")).toContain("136px");

    fireEvent.lostPointerCapture(resizer, { pointerId: 7 });

    await waitFor(() =>
      expect(table.querySelector("col")?.getAttribute("style")).toContain("120px"),
    );
    expect(onChange).not.toHaveBeenCalled();
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-001.10
describe("HybridMarkdownEditor recovery boundary", () => {
  it.each([undefined, () => {}])(
    "mounts the engine when EditContext is unavailable or nonconstructible: %s",
    (capability) => {
      vi.stubGlobal("EditContext", capability);
      const onSourceFallback = vi.fn();
      render(
        <HybridMarkdownEditor
          content="# preserved\r\n"
          onChange={vi.fn()}
          onSourceFallback={onSourceFallback}
        />,
      );
      expect(upstream.state.models).toHaveLength(1);
      expect(onSourceFallback).not.toHaveBeenCalled();
    },
  );

  it("disposes once after update failure and ignores queued and late edits", async () => {
    const diagnostic = vi.spyOn(console, "warn").mockImplementation(() => {});
    const onChange = vi.fn();
    const onSourceFallback = vi.fn();
    const { rerender, unmount } = render(
      <HybridMarkdownEditor
        content="# initial"
        onChange={onChange}
        onSourceFallback={onSourceFallback}
      />,
    );
    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    model.replaceSourceText.mockImplementation(() => {
      throw new Error("private document text");
    });
    model.applyUserEdit({
      replacements: [{ replaceRange: { start: 0, endExclusive: 0 }, newText: "queued" }],
    });
    rerender(
      <HybridMarkdownEditor
        content="# authoritative"
        onChange={onChange}
        onSourceFallback={onSourceFallback}
      />,
    );
    model.applyUserEdit({
      replacements: [{ replaceRange: { start: 0, endExclusive: 0 }, newText: "late" }],
    });
    await Promise.resolve();
    expect(onChange).not.toHaveBeenCalled();
    expect(onSourceFallback).toHaveBeenCalledOnce();
    expect(screen.getByTestId("hybrid-markdown-editor").childElementCount).toBe(0);
    unmount();
    expect(model.dispose).toHaveBeenCalledOnce();
    expect(view.dispose).toHaveBeenCalledOnce();
    expect(diagnostic.mock.calls).toEqual([
      [
        "hybrid_markdown_editor_failure",
        { fileType: "markdown", mode: "edit", operation: "update", upstreamVersion: "0.0.2-84" },
      ],
    ]);
    diagnostic.mockRestore();
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
describe("editable hybrid comments", () => {
  it("captures a user selection before native comment input focus and preserves the source", async () => {
    const onComment = vi.fn();
    const onChange = vi.fn();
    render(
      <HybridMarkdownEditor
        content="# Heading\n\nBody"
        onChange={onChange}
        onComment={onComment}
      />,
    );
    const model = upstream.state.models[0];
    await act(async () =>
      model.selection.set({ isCollapsed: false, range: { start: 2, endExclusive: 9 } }),
    );
    fireEvent.click(screen.getByTestId("hybrid-markdown-comment-trigger"));
    await act(async () => model.selection.set(undefined));
    const textarea = screen.getByRole("textbox");
    expect(screen.getByTestId("hybrid-markdown-editor").contains(textarea)).toBe(false);
    fireEvent.change(textarea, { target: { value: "Check heading" } });
    fireEvent.keyDown(textarea, { key: "Enter", ctrlKey: true });
    expect(onComment).toHaveBeenCalledWith({ text: "Check heading", start: 2, endExclusive: 9 });
    expect(onChange).not.toHaveBeenCalled();
    expect(model.readonlyMode.get()).toBe(false);
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-001.5
describe("HybridMarkdownEditor source position port", () => {
  it("restores reversed source selection with the public range reveal and cancels on hide", () => {
    const callback = vi.fn();
    const props = {
      content: "one\r\ntwo\r\nthree",
      onChange: vi.fn(),
      onSourcePositionHandle: callback,
    };
    const { rerender, unmount } = render(<HybridMarkdownEditor {...props} isActive={false} />);
    expect(callback).not.toHaveBeenCalled();
    rerender(<HybridMarkdownEditor {...props} isActive />);
    const handle = callback.mock.lastCall![0] as SourcePositionHandle;
    act(() => handle.restore({ selection: { anchor: 12, active: 6 }, nearestLine: 2 }));
    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    expect(model.selection.get()).toMatchObject({ anchor: 12, active: 6 });
    expect(handle.capture()).toEqual({ selection: { anchor: 12, active: 6 }, nearestLine: 2 });
    expect(view.revealRangeInCenterIfOutsideViewport).toHaveBeenCalledWith(
      expect.objectContaining({ start: 6, endExclusive: 12 }),
      "instant",
    );
    rerender(<HybridMarkdownEditor {...props} isActive={false} />);
    expect(view.pendingReveal.dispose).toHaveBeenCalledOnce();
    view.focus.mockClear();
    handle.restore({ nearestLine: 1 });
    expect(view.focus).not.toHaveBeenCalled();
    unmount();
  });

  it("clamps selection after accepted replacement and cancels reveals on replacement and failure", () => {
    const callback = vi.fn();
    const props = {
      content: "one\r\ntwo\r\nthree",
      onChange: vi.fn(),
      onSourcePositionHandle: callback,
    };
    const { rerender } = render(<HybridMarkdownEditor {...props} />);
    const handle = callback.mock.lastCall![0] as SourcePositionHandle;
    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    act(() => handle.restore({ selection: { anchor: 12, active: 6 }, nearestLine: 2 }));
    rerender(<HybridMarkdownEditor {...props} content="xy" />);
    expect(model.selection.get()).toMatchObject({ anchor: 2, active: 2 });
    expect(view.pendingReveal.dispose).toHaveBeenCalledOnce();
    expect(callback).toHaveBeenCalledTimes(1);
    act(() => handle.restore({ selection: { anchor: 1, active: 0 }, nearestLine: 1 }));
    model.replaceSourceText.mockImplementation(() => {
      throw new Error("private source");
    });
    rerender(<HybridMarkdownEditor {...props} content="failed replacement" />);
    expect(view.pendingReveal.dispose).toHaveBeenCalledTimes(2);
    view.focus.mockClear();
    handle.restore({ nearestLine: 1 });
    expect(view.focus).not.toHaveBeenCalled();
  });
});
