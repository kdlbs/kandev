import { vi } from "vitest";

export function installTableGeometry(table: HTMLTableElement) {
  const tableRect = { left: 48, top: 48, width: 240, height: 120, right: 288, bottom: 168 };
  const wrapper = table.parentElement as HTMLDivElement;
  Object.defineProperty(wrapper, "getBoundingClientRect", { value: () => tableRect });
  Object.defineProperty(table, "getBoundingClientRect", { value: () => tableRect });
  Array.from(table.rows).forEach((row, rowIndex) => {
    Object.defineProperty(row, "getBoundingClientRect", {
      value: () => ({
        ...tableRect,
        top: 48 + rowIndex * 40,
        bottom: 88 + rowIndex * 40,
        height: 40,
      }),
    });
    Array.from(row.cells).forEach((cell, cellIndex) => {
      Object.defineProperty(cell, "getBoundingClientRect", {
        value: () => ({
          ...tableRect,
          left: 48 + cellIndex * 120,
          right: 168 + cellIndex * 120,
          width: 120,
          top: 48 + rowIndex * 40,
          bottom: 88 + rowIndex * 40,
          height: 40,
        }),
      });
    });
  });
}

// The faithful upstream lifecycle mock keeps its model, view, and controller state together.
// eslint-disable-next-line max-lines-per-function
export const upstream = (() => {
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
    const observers = new Set<{ handleChange: () => void; endUpdate: () => void }>();
    return {
      addObserver: (observer: { handleChange: () => void; endUpdate: () => void }) =>
        observers.add(observer),
      removeObserver: (observer: { handleChange: () => void; endUpdate: () => void }) =>
        observers.delete(observer),
      get: vi.fn(() => value),
      set: vi.fn((next: T) => {
        value = next;
        for (const observer of observers) {
          observer.handleChange();
          observer.endUpdate();
        }
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
    selectionSource = observable("user");
    isSelecting = observable(false);
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
    caretRect = observable({ left: 10, top: 20, right: 10, bottom: 40 });
    focus = vi.fn();
    pendingReveal = { dispose: vi.fn() };
    revealRangeInCenterIfOutsideViewport = vi.fn(() => this.pendingReveal);
    measuredLayout = { visualLineMap: { get: () => ({ isEmpty: true }) } };
    coordinateSpace = { capture: () => ({ toClientRect: (rect: unknown) => rect }) };
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
    constructor(
      readonly anchor: number,
      readonly active: number,
    ) {}
    get isCollapsed() {
      return this.anchor === this.active;
    }
    get range() {
      return new MockOffsetRange(
        Math.min(this.anchor, this.active),
        Math.max(this.anchor, this.active),
      );
    }
    static collapsed(offset: number) {
      return { anchor: offset, active: offset };
    }
  }

  function setupActiveTable(
    model: MockEditorModel,
    view: MockEditorView,
    source: string,
    bodyRows: string,
    columnCount = 2,
  ) {
    const table = {
      kind: "table",
      length: source.length,
      headerRow: { cells: Array.from({ length: columnCount }, () => ({})) },
    };
    model.document.set({ content: [table] });
    model.activeBlock.set(table);
    const header = Array.from({ length: columnCount }, (_, index) => `<th>${index + 1}</th>`).join(
      "",
    );
    const delimiter = Array.from({ length: columnCount }, () => "<td>---</td>").join("");
    view.element.innerHTML = `
      <div class="md-table-wrapper">
        <table class="md-table md-block-active">
          <tr>${header}</tr>
          <tr class="md-table-delimiter-row">${delimiter}</tr>
          ${bodyRows}
        </table>
      </div>`;
    return table;
  }

  return {
    state,
    EditorModel: MockEditorModel,
    EditorView: MockEditorView,
    EditorController: MockEditorController,
    LocalHistoryStrategy: MockLocalHistoryStrategy,
    StringEdit: MockStringEdit,
    Selection: MockSelection,
    setupActiveTable,
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
})();
