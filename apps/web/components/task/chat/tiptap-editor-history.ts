import { useMemo } from "react";
import type { useEditor } from "@tiptap/react";
import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Node as ProseMirrorNode } from "@tiptap/pm/model";
import { navigateHistory, type HistoryState } from "./message-history";
import { getMarkdownText, textToHtml } from "./tiptap-helpers";

export type HistoryNavDecision =
  | { kind: "defer" }
  | { kind: "consume-noop" }
  | { kind: "apply"; index: number | null };

/** Pure decision for whether ArrowUp/ArrowDown should engage message history
 *  navigation. Kept pure so the keymap contract is unit-testable without
 *  mounting TipTap. `atBoundary` is true when the caret is at the top of the
 *  first textblock (for "up") or the bottom of the last textblock (for
 *  "down") — pressing the arrow elsewhere should defer to normal cursor
 *  movement. */
export function decideHistoryNav(args: {
  direction: "up" | "down";
  disabled: boolean;
  isSuggestionMenuOpen: boolean;
  isReverseSearchOpen: boolean;
  atBoundary: boolean;
  historyLength: number;
  state: HistoryState;
}): HistoryNavDecision {
  if (args.disabled) return { kind: "defer" };
  if (args.isSuggestionMenuOpen) return { kind: "defer" };
  if (args.isReverseSearchOpen) return { kind: "defer" };
  if (args.historyLength === 0) return { kind: "defer" };
  if (!args.atBoundary) return { kind: "defer" };
  const next = navigateHistory(args.state, args.direction, args.historyLength);
  if (next === null) {
    if (args.direction === "up") return { kind: "consume-noop" };
    return { kind: "defer" };
  }
  return { kind: "apply", index: next.index };
}

export type HistoryKeymapRefs = {
  disabledRef: React.RefObject<boolean | undefined>;
  isSuggestionMenuOpenRef: React.RefObject<boolean>;
  isReverseSearchOpenRef: React.RefObject<boolean>;
  getHistoryRef: React.RefObject<() => readonly string[]>;
  onOpenReverseSearchRef: React.RefObject<(() => void) | undefined>;
  onChangeRef: React.RefObject<(value: string) => void>;
};

export type HistoryNavController = {
  extension: Extension;
  /** Apply a specific history entry by index. Used by the reverse-search
   *  overlay so the picked entry slots back into the up/down cycle. */
  applyHistoryIndex: (editor: ReturnType<typeof useEditor> | null, index: number) => void;
};

type Editor = ReturnType<typeof useEditor>;

type HistoryRuntimeState = {
  index: number | null;
  draft: string;
  isApplying: boolean;
};

function writeHistoryContent(
  editor: Editor,
  state: HistoryRuntimeState,
  text: string,
  onChange: (value: string) => void,
): void {
  if (!editor) return;
  state.isApplying = true;
  try {
    if (text === "") {
      editor.chain().focus().clearContent().run();
    } else {
      editor.chain().focus().setContent(textToHtml(text)).run();
      editor.commands.focus("end");
    }
    onChange(getMarkdownText(editor));
  } finally {
    state.isApplying = false;
  }
}

function caretAtBoundary(editor: Editor, direction: "up" | "down"): boolean {
  if (!editor) return false;
  const view = editor.view;
  const { selection, doc } = editor.state;
  const parent = selection.$from.parent as ProseMirrorNode;
  if (direction === "up") {
    return parent === doc.firstChild && view.endOfTextblock("up");
  }
  return parent === doc.lastChild && view.endOfTextblock("down");
}

function runHistoryArrow(
  editor: Editor,
  direction: "up" | "down",
  state: HistoryRuntimeState,
  refs: HistoryKeymapRefs,
): boolean {
  if (!editor) return false;
  const history = refs.getHistoryRef.current?.() ?? [];
  const decision = decideHistoryNav({
    direction,
    disabled: !!refs.disabledRef.current,
    isSuggestionMenuOpen: refs.isSuggestionMenuOpenRef.current,
    isReverseSearchOpen: refs.isReverseSearchOpenRef.current,
    atBoundary: caretAtBoundary(editor, direction),
    historyLength: history.length,
    state: { index: state.index },
  });
  if (decision.kind === "defer") return false;
  if (decision.kind === "consume-noop") return true;
  if (state.index === null && decision.index !== null) {
    state.draft = getMarkdownText(editor);
  }
  const text = decision.index === null ? state.draft : history[decision.index];
  writeHistoryContent(editor, state, text, refs.onChangeRef.current);
  state.index = decision.index;
  return true;
}

function reverseSearchPlugin(state: HistoryRuntimeState, refs: HistoryKeymapRefs): Plugin {
  return new Plugin({
    key: new PluginKey("messageHistoryReverseSearch"),
    props: {
      handleKeyDown: (_view, event) => {
        if (refs.disabledRef.current) return false;
        if (event.ctrlKey && !event.metaKey && !event.altKey && event.key.toLowerCase() === "r") {
          event.preventDefault();
          refs.onOpenReverseSearchRef.current?.();
          return true;
        }
        return false;
      },
    },
    filterTransaction: (tr) => {
      if (tr.docChanged && !state.isApplying && state.index !== null) {
        state.index = null;
        state.draft = "";
      }
      return true;
    },
  });
}

function buildHistoryExtension(state: HistoryRuntimeState, refs: HistoryKeymapRefs): Extension {
  return Extension.create({
    name: "messageHistoryKeymap",
    addKeyboardShortcuts() {
      return {
        ArrowUp: ({ editor }) => runHistoryArrow(editor, "up", state, refs),
        ArrowDown: ({ editor }) => runHistoryArrow(editor, "down", state, refs),
      };
    },
    addProseMirrorPlugins() {
      return [reverseSearchPlugin(state, refs)];
    },
  });
}

function applyHistoryIndexImpl(
  editor: Editor | null,
  state: HistoryRuntimeState,
  index: number,
  refs: HistoryKeymapRefs,
): void {
  if (!editor) return;
  const history = refs.getHistoryRef.current?.() ?? [];
  if (index < 0 || index >= history.length) return;
  if (state.index === null) state.draft = getMarkdownText(editor);
  writeHistoryContent(editor, state, history[index], refs.onChangeRef.current);
  state.index = index;
}

export function useHistoryKeymap(refs: HistoryKeymapRefs): HistoryNavController {
  return useMemo(() => {
    const state: HistoryRuntimeState = { index: null, draft: "", isApplying: false };
    const extension = buildHistoryExtension(state, refs);
    const applyHistoryIndex: HistoryNavController["applyHistoryIndex"] = (editor, index) =>
      applyHistoryIndexImpl(editor, state, index, refs);
    return { extension, applyHistoryIndex };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}
