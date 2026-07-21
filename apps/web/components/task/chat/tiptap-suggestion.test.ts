import { describe, expect, it, vi } from "vitest";
import type { MentionItem } from "@/hooks/use-inline-mention";
import type { EntityReference } from "@/lib/types/entity-reference";
import {
  createEntityReferenceSuggestion,
  handleEntityReferenceMenuKeyDown,
} from "./tiptap-entity-reference-suggestion";
import { createMentionSuggestion } from "./tiptap-suggestion";
import * as entityReferenceSuggestions from "./tiptap-entity-reference-suggestion";

describe("entity reference suggestion", () => {
  it("provides an independent # suggestion config", () => {
    expect(
      typeof (entityReferenceSuggestions as Record<string, unknown>)
        .createEntityReferenceSuggestion,
    ).toBe("function");
  });

  it("provides keyboard navigation for grouped reference results", () => {
    expect(
      typeof (entityReferenceSuggestions as Record<string, unknown>)
        .handleEntityReferenceMenuKeyDown,
    ).toBe("function");
  });

  it("uses arrows to move and Enter or Tab to select without submitting", () => {
    const handle = handleEntityReferenceMenuKeyDown as unknown as (args: {
      event: KeyboardEvent;
      items: EntityReference[];
      selectedIndex: number;
      setSelectedIndex: (index: number) => void;
      onSelect: (reference: EntityReference) => void;
    }) => boolean;
    const items = [
      {
        version: 1,
        ref: "mention:v1:kandev:task:task-1",
        provider: "kandev",
        kind: "task",
        id: "task-1",
        title: "First task",
        url: "/t/task-1",
        scope: "workspace-1",
      },
      {
        version: 1,
        ref: "mention:v1:kandev:task:task-2",
        provider: "kandev",
        kind: "task",
        id: "task-2",
        title: "Second task",
        url: "/t/task-2",
        scope: "workspace-1",
      },
    ];
    const setSelectedIndex = vi.fn();
    const onSelect = vi.fn();

    expect(
      handle({
        event: new KeyboardEvent("keydown", { key: "ArrowDown" }),
        items,
        selectedIndex: 0,
        setSelectedIndex,
        onSelect,
      }),
    ).toBe(true);
    expect(setSelectedIndex).toHaveBeenCalledWith(1);
    expect(
      handle({
        event: new KeyboardEvent("keydown", { key: "Tab" }),
        items,
        selectedIndex: 1,
        setSelectedIndex,
        onSelect,
      }),
    ).toBe(true);
    expect(onSelect).toHaveBeenCalledWith(items[1]);
  });
});

describe("entity reference suggestion lifecycle", () => {
  it("replaces only the active # range with an atom and trailing space", () => {
    const suggestion = createEntityReferenceSuggestion(vi.fn(), vi.fn());
    const run = vi.fn();
    const insertContentAt = vi.fn(() => ({ run }));
    const focus = vi.fn(() => ({ insertContentAt }));
    const editor = { chain: () => ({ focus }) };
    const reference: EntityReference = {
      version: 1,
      ref: "mention:v1:github:pull_request:github.com/acme/app:42",
      provider: "github",
      kind: "pull_request",
      id: "MDExOlB1bGxSZXF1ZXN0NDI=",
      key: "42",
      title: "Fix authentication",
      url: "https://github.com/acme/app/pull/42",
      scope: "github.com/acme/app",
    };

    expect(typeof suggestion.command).toBe("function");
    suggestion.command?.({
      editor: editor as never,
      range: { from: 8, to: 13 },
      props: reference,
    });

    expect(insertContentAt).toHaveBeenCalledWith({ from: 8, to: 13 }, [
      { type: "entityReference", attrs: reference },
      { type: "text", text: " " },
    ]);
    expect(run).toHaveBeenCalledOnce();
  });

  it("opens in the primed state for bare # and Escape closes without selecting", () => {
    const setMenuState = vi.fn();
    const suggestion = createEntityReferenceSuggestion(setMenuState, vi.fn());
    const lifecycle = suggestion.render?.();
    const props = {
      editor: {} as never,
      range: { from: 1, to: 2 },
      query: "",
      text: "#",
      items: [],
      command: vi.fn(),
      decorationNode: null,
      clientRect: () => new DOMRect(10, 20, 1, 10),
    };

    expect(lifecycle).toBeDefined();
    lifecycle?.onStart?.(props);
    expect(setMenuState).toHaveBeenLastCalledWith(
      expect.objectContaining({ isOpen: true, query: "", items: [] }),
    );

    const handled = lifecycle?.onKeyDown?.({
      view: {} as never,
      event: new KeyboardEvent("keydown", { key: "Escape" }),
      range: props.range,
    });
    expect(handled).toBe(true);
    expect(setMenuState).toHaveBeenLastCalledWith({
      isOpen: false,
      items: [],
      query: "",
      clientRect: null,
      command: null,
    });
    expect(props.command).not.toHaveBeenCalled();
  });

  it("allows # at a text-block boundary but rejects tokens and code", () => {
    const suggestion = createEntityReferenceSuggestion(vi.fn(), vi.fn());
    const allowed = (prefix: string, parentType = "paragraph", hasCodeMark = false) => {
      const resolved = {
        parent: {
          type: { name: parentType },
          textBetween: () => prefix,
        },
        start: () => 1,
        marks: () => (hasCodeMark ? [{ type: { name: "code" } }] : []),
      };
      return suggestion.allow?.({
        editor: {} as never,
        state: { doc: { resolve: () => resolved } } as never,
        range: { from: prefix.length + 1, to: prefix.length + 2 },
      });
    };

    expect(typeof suggestion.allow).toBe("function");
    expect(allowed("")).toBe(true);
    expect(allowed("Discuss ")).toBe(true);
    expect(allowed("issue")).toBe(false);
    expect(allowed("", "codeBlock")).toBe(false);
    expect(allowed("", "paragraph", true)).toBe(false);
  });
});

describe("createMentionSuggestion", () => {
  it("keeps new task discovery out of the @ menu", async () => {
    const file: MentionItem = {
      id: "src/app.ts",
      kind: "file",
      label: "src/app.ts",
      onSelect: vi.fn(),
    };
    const task: MentionItem = {
      id: "task:task-2",
      kind: "task",
      label: "Follow-up task",
      task: {
        taskId: "task-2",
        title: "Follow-up task",
        workflowId: "workflow-1",
        workflowStepId: "step-1",
        state: null,
      },
      onSelect: vi.fn(),
    };
    const suggestion = createMentionSuggestion(
      { getItems: vi.fn().mockResolvedValue([task, file]) },
      vi.fn(),
      vi.fn(),
    );

    const items = await (suggestion.items as (args: { query: string }) => Promise<MentionItem[]>)({
      query: "",
    });

    expect(items).toEqual([file]);
  });
});
