import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// The faithful upstream lifecycle mock keeps its model, view, and controller state together.
// eslint-disable-next-line max-lines-per-function
const upstream = vi.hoisted(() => {
  type SourceReplacement = {
    replaceRange: { start: number; endExclusive: number };
    newText: string;
  };
  type SourceEdit = { replacements: readonly SourceReplacement[] };
  type SourceEditListener = (event: { edit: SourceEdit }) => void;
  const state: {
    models: MockEditorModel[];
    views: MockEditorView[];
    controllers: MockEditorController[];
    histories: MockLocalHistoryStrategy[];
    syntaxHighlighters: { dispose: ReturnType<typeof vi.fn> }[];
    failView: boolean;
  } = {
    models: [],
    views: [],
    controllers: [],
    histories: [],
    syntaxHighlighters: [],
    failView: false,
  };

  function observable<T>(initial: T) {
    let value = initial;
    return {
      get: vi.fn(() => value),
      set: vi.fn((next: T) => {
        value = next;
      }),
    };
  }

  class MockStringValue {
    constructor(readonly value: string) {}
  }

  class MockOffsetRange {
    constructor(
      readonly start: number,
      readonly endExclusive: number,
    ) {}
  }

  class MockEditorModel {
    sourceText = observable(new MockStringValue(""));
    readonlyMode = observable(false);
    baseline = observable<MockStringValue | undefined>(undefined);
    gutterMarkers = observable<readonly unknown[]>([]);
    document = observable<{ content: readonly { kind: string; length: number }[] }>({
      content: [],
    });
    activeBlock = observable<
      { kind: string; length: number; headerRow?: { cells: unknown[] } } | undefined
    >(undefined);
    selection = observable<unknown>(undefined);
    listener: SourceEditListener | undefined;
    sourceEditSubscription = { dispose: vi.fn() };
    replaceSourceText = vi.fn((value: MockStringValue) => this.sourceText.set(value));
    onWillApplySourceEdit = vi.fn((listener: SourceEditListener) => {
      this.listener = listener;
      return this.sourceEditSubscription;
    });
    applyUserEdit(edit: SourceEdit) {
      this.listener?.({ edit });
      const source = this.sourceText.get().value;
      const nextSource = [...edit.replacements]
        .sort((left, right) => right.replaceRange.start - left.replaceRange.start)
        .reduce(
          (next, replacement) =>
            `${next.slice(0, replacement.replaceRange.start)}${replacement.newText}${next.slice(
              replacement.replaceRange.endExclusive,
            )}`,
          source,
        );
      this.sourceText.set(new MockStringValue(nextSource));
    }
    applyEdit = vi.fn((edit: SourceEdit, selection?: unknown) => {
      this.applyUserEdit(edit);
      this.selection.set(selection);
    });
    dispose = vi.fn();

    constructor() {
      state.models.push(this);
    }
  }

  class MockEditorView {
    element = document.createElement("div");
    dispose = vi.fn();
    options: unknown;

    constructor(_model: MockEditorModel, options: unknown) {
      if (state.failView) throw new Error("view failed");
      this.options = options;
      state.views.push(this);
    }
  }

  class MockEditorController {
    dispose = vi.fn();

    constructor(
      readonly model: MockEditorModel,
      readonly view: MockEditorView,
      readonly options: unknown,
    ) {
      state.controllers.push(this);
    }
  }

  class MockCommentsModel {
    set = vi.fn();
  }

  class MockCommentsView {
    dispose = vi.fn();
  }

  class MockCommentModeController {
    dispose = vi.fn();
  }

  class MockLocalHistoryStrategy {
    record = vi.fn((operation: () => void) => operation());

    constructor(readonly model: MockEditorModel) {
      state.histories.push(this);
    }
  }

  class MockStringEdit {
    static replace(replaceRange: { start: number; endExclusive: number }, newText: string) {
      return { replacements: [{ replaceRange, newText }] };
    }
  }

  class MockSelection {
    static collapsed(offset: number) {
      return { anchor: offset, active: offset };
    }
  }

  return {
    state,
    EditorModel: MockEditorModel,
    EditorView: MockEditorView,
    EditorController: MockEditorController,
    LocalHistoryStrategy: MockLocalHistoryStrategy,
    StringEdit: MockStringEdit,
    Selection: MockSelection,
    createDefaultMonacoSyntaxHighlighter: vi.fn(() => {
      const highlighter = { dispose: vi.fn(), create: vi.fn() };
      state.syntaxHighlighters.push(highlighter);
      return highlighter;
    }),
    StringValue: MockStringValue,
    OffsetRange: MockOffsetRange,
    CommentsModel: MockCommentsModel,
    CommentsView: MockCommentsView,
    CommentModeController: MockCommentModeController,
  };
});

vi.mock("@vscode/markdown-editor", () => upstream);
vi.mock("@vscode/markdown-editor/editor.css", () => ({}));
vi.mock("@vscode/markdown-editor/themes/default.css", () => ({}));

import { HybridMarkdownEditor } from "./hybrid-markdown-editor";

afterEach(cleanup);

beforeEach(() => {
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
  it("appends rows and columns through local history when a table is active", async () => {
    const source = "| A | B |\n| --- | --- |\n| 1 | 2 |\n";
    const onChange = vi.fn();
    render(<HybridMarkdownEditor content={source} readOnly={false} onChange={onChange} />);

    const model = upstream.state.models[0];
    const view = upstream.state.views[0];
    const table = { kind: "table", length: source.length, headerRow: { cells: [{}, {}] } };
    model.document.set({ content: [table] });
    model.activeBlock.set(table);
    view.element.innerHTML =
      '<div class="md-table-wrapper"><table class="md-table md-block-active"></table></div>';

    const addRow = await screen.findByRole("button", { name: "Add row below" });
    fireEvent.click(addRow);

    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith("| A | B |\n| --- | --- |\n| 1 | 2 |\n|  |  |\n"),
    );
    expect(upstream.state.histories[0].record).toHaveBeenCalledOnce();

    model.sourceText.set(new upstream.StringValue(source));
    model.document.set({ content: [table] });
    model.activeBlock.set(table);
    fireEvent.click(screen.getByRole("button", { name: "Add column right" }));

    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith(
        "| A | B |  |\n| --- | --- | --- |\n| 1 | 2 |  |\n",
      ),
    );
    expect(upstream.state.histories[0].record).toHaveBeenCalledTimes(2);
  });

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

    expect(onError).toHaveBeenCalledWith(expect.objectContaining({ message: "view failed" }));
    expect(onSourceFallback).toHaveBeenCalledOnce();
    expect(upstream.state.models[0].dispose).toHaveBeenCalledOnce();
  });
});
