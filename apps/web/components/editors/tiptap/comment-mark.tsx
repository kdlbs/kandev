/**
 * TipTap Mark extension for plan comments.
 *
 * Uses a native Mark (like the Highlight extension) for the background —
 * marks are part of the ProseMirror document model, so deleting marked text
 * automatically kills the mark ("zombie comment" fix).
 *
 * A single badge per comment is placed via a widget decoration at the end
 * of the mark range, avoiding the per-text-node duplication that
 * ReactMarkViewRenderer caused on multi-line selections.
 */

import { Mark, mergeAttributes } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type CommentForEditor = {
  id: string;
  selectedText: string;
  from?: number;
  to?: number;
};

type CommentMarkOptions = {
  onOrphanedComments: (ids: string[]) => void;
  onCommentClick: (id: string, position: { x: number; y: number }) => void;
};

// ---------------------------------------------------------------------------
// Text-search utilities (moved from tiptap-comment-highlight.ts)
// ---------------------------------------------------------------------------

export const MIN_COMMENT_TEXT_LENGTH = 3;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type DocLike = any;

function getTextWithPositions(doc: DocLike): { text: string; positions: number[] } {
  let text = "";
  const positions: number[] = [];

  doc.descendants(
    (
      node: { isText: boolean; text?: string; type: { name: string }; isBlock: boolean },
      pos: number,
    ) => {
      if (node.isText && node.text) {
        for (let i = 0; i < node.text.length; i++) {
          positions.push(pos + i);
          text += node.text[i];
        }
      } else if (node.type.name === "hardBreak") {
        positions.push(-1);
        text += " ";
      } else if (node.isBlock && text.length > 0 && !text.endsWith("\n")) {
        positions.push(-1);
        text += "\n";
      }
    },
  );

  return { text, positions };
}

function normalizeForSearch(text: string): string {
  return text
    .replace(/\r\n/g, "\n")
    .replace(/[\t ]+/g, " ")
    .replace(/\n+/g, "\n")
    .trim()
    .toLowerCase();
}

function buildNormToOrigMap(fullText: string, normalizedFullText: string): number[] {
  const normalizedChars = normalizedFullText.split("");
  const origChars = fullText.split("");
  const normToOrig: number[] = [];
  let oi = 0;
  for (let ni = 0; ni < normalizedChars.length; ni++) {
    while (
      oi < origChars.length &&
      origChars[oi].toLowerCase() !== normalizedChars[ni] &&
      /\s/.test(origChars[oi])
    ) {
      oi++;
    }
    normToOrig[ni] = oi;
    oi++;
  }
  return normToOrig;
}

function resolveTextSearchPositions(
  normalizedIndex: number,
  length: number,
  fullText: string,
  normalizedFullText: string,
  positions: number[],
): { from: number; to: number } | null {
  const normToOrig = buildNormToOrigMap(fullText, normalizedFullText);
  const startOrig = normToOrig[normalizedIndex] ?? 0;
  const endOrig = normToOrig[normalizedIndex + length - 1] ?? startOrig;

  let from = -1;
  let to = -1;
  for (let i = startOrig; i <= endOrig && i < positions.length; i++) {
    if (positions[i] >= 0) {
      if (from === -1) from = positions[i];
      to = positions[i] + 1;
    }
  }
  if (from === -1 || to === -1) return null;
  return { from, to };
}

function findCommentByTextSearch(
  comment: CommentForEditor,
  fullText: string,
  normalizedFullText: string,
  positions: number[],
): { from: number; to: number } | null {
  const searchText = normalizeForSearch(comment.selectedText);
  if (!searchText || searchText.length < MIN_COMMENT_TEXT_LENGTH) return null;

  let normalizedIndex = normalizedFullText.indexOf(searchText);
  if (normalizedIndex !== -1) {
    return resolveTextSearchPositions(
      normalizedIndex,
      searchText.length,
      fullText,
      normalizedFullText,
      positions,
    );
  }

  const firstLine = normalizeForSearch(comment.selectedText.split("\n")[0]);
  if (firstLine.length >= MIN_COMMENT_TEXT_LENGTH) {
    normalizedIndex = normalizedFullText.indexOf(firstLine);
    if (normalizedIndex !== -1) {
      return resolveTextSearchPositions(
        normalizedIndex,
        firstLine.length,
        fullText,
        normalizedFullText,
        positions,
      );
    }
  }

  return null;
}

// ---------------------------------------------------------------------------
// Helpers: collect mark IDs from a doc
// ---------------------------------------------------------------------------

function collectCommentIds(doc: DocLike, markTypeName: string): Set<string> {
  const ids = new Set<string>();
  doc.descendants(
    (node: { marks?: Array<{ type: { name: string }; attrs: { commentId?: string } }> }) => {
      if (node.marks) {
        for (const mark of node.marks) {
          if (mark.type.name === markTypeName && mark.attrs.commentId) {
            ids.add(mark.attrs.commentId);
          }
        }
      }
    },
  );
  return ids;
}

// ---------------------------------------------------------------------------
// Badge widget — single DOM element per comment, placed via decoration
// ---------------------------------------------------------------------------

function createCommentBadge(commentId: string): HTMLSpanElement {
  const badge = document.createElement("span");
  badge.className = "comment-badge";
  badge.setAttribute("data-comment-id", commentId);
  badge.innerHTML =
    '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
    'stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
    '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>';
  return badge;
}

/** Build one widget decoration per commentId at the end of its mark range. */
function buildBadgeDecorations(doc: DocLike, markTypeName: string): DecorationSet {
  const lastPos = new Map<string, number>();

  doc.descendants(
    (
      node: {
        marks?: Array<{ type: { name: string }; attrs: { commentId?: string } }>;
        nodeSize: number;
      },
      pos: number,
    ) => {
      if (!node.marks) return;
      for (const mark of node.marks) {
        if (mark.type.name === markTypeName && mark.attrs.commentId) {
          const end = pos + node.nodeSize;
          const cur = lastPos.get(mark.attrs.commentId);
          if (!cur || end > cur) lastPos.set(mark.attrs.commentId, end);
        }
      }
    },
  );

  const decos: Decoration[] = [];
  for (const [commentId, endPos] of lastPos) {
    decos.push(
      Decoration.widget(endPos, () => createCommentBadge(commentId), {
        side: 1,
        key: `badge-${commentId}`,
      }),
    );
  }
  return DecorationSet.create(doc, decos);
}

// ---------------------------------------------------------------------------
// Plugin keys
// ---------------------------------------------------------------------------

const orphanPluginKey = new PluginKey("commentMark-orphanDetection");
const badgePluginKey = new PluginKey("commentMark-badges");

// ---------------------------------------------------------------------------
// Helper: find commentId at a document position
// ---------------------------------------------------------------------------

type MarkEntry = { type: { name: string }; attrs: { commentId?: string } };

function commentIdAtPos(doc: DocLike, pos: number, markTypeName: string): string | null {
  if (pos < 0 || pos >= doc.content.size) return null;
  const $pos = doc.resolve(pos);
  const after = $pos.parent.childAfter($pos.parentOffset);
  if (!after.node) return null;
  const mark = (after.node.marks as MarkEntry[]).find(
    (m) => m.type.name === markTypeName && m.attrs.commentId,
  );
  return mark?.attrs.commentId ?? null;
}

// ---------------------------------------------------------------------------
// Mark Extension
// ---------------------------------------------------------------------------

export const CommentMark = Mark.create<CommentMarkOptions>({
  name: "commentMark",

  // Coexist with bold, italic, highlight, etc.
  excludes: "",

  // Typing at the edge of a mark should NOT extend it
  inclusive: false,

  addOptions() {
    return {
      onOrphanedComments: () => {},
      onCommentClick: () => {},
    };
  },

  addAttributes() {
    return {
      commentId: { default: null },
    };
  },

  parseHTML() {
    return [{ tag: "span[data-comment-id]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes(
        { class: "comment-highlight", "data-comment-id": HTMLAttributes.commentId },
        HTMLAttributes,
      ),
      0,
    ];
  },

  // Transparent in markdown — just outputs the inner text
  addStorage() {
    return {
      markdown: {
        serialize: { open: "", close: "", mixable: true, expelEnclosingWhitespace: true },
        parse: {},
      },
    };
  },

  // Backspace/Delete inside a comment mark removes the entire mark (keeps text).
  // Orphan detection then cleans up the comment from the store.
  addKeyboardShortcuts() {
    const markName = this.name;

    /** Remove all instances of commentMark with the given ID from the doc. */
    const removeCommentById = (commentId: string): boolean => {
      const { state } = this.editor;
      const type = state.schema.marks[markName];
      const markToRemove = type.create({ commentId });
      const { tr } = state;
      tr.removeMark(0, state.doc.content.size, markToRemove);
      this.editor.view.dispatch(tr);
      return true;
    };

    return {
      Backspace: () => {
        const { selection } = this.editor.state;
        if (!selection.empty) return false;
        // Backspace targets the char at pos-1
        const id = commentIdAtPos(this.editor.state.doc, selection.$from.pos - 1, markName);
        if (!id) return false;
        return removeCommentById(id);
      },
      Delete: () => {
        const { selection } = this.editor.state;
        if (!selection.empty) return false;
        // Delete targets the char at pos
        const id = commentIdAtPos(this.editor.state.doc, selection.$from.pos, markName);
        if (!id) return false;
        return removeCommentById(id);
      },
    };
  },

  addProseMirrorPlugins() {
    const markName = this.name;
    const { onOrphanedComments } = this.options;

    return [
      // Single badge per commentId at the end of its range
      new Plugin({
        key: badgePluginKey,
        state: {
          init(_, state) {
            return buildBadgeDecorations(state.doc, markName);
          },
          apply(tr, value, _, newState) {
            if (tr.docChanged) return buildBadgeDecorations(newState.doc, markName);
            return value;
          },
        },
        props: {
          decorations(state) {
            return badgePluginKey.getState(state) ?? DecorationSet.empty;
          },
        },
      }),
      // Orphan detection — fire callback when a commentId disappears from the doc
      new Plugin({
        key: orphanPluginKey,
        appendTransaction(transactions, oldState, newState) {
          const docChanged = transactions.some((tr) => tr.docChanged);
          if (!docChanged) return null;

          const oldIds = collectCommentIds(oldState.doc, markName);
          const newIds = collectCommentIds(newState.doc, markName);

          const orphaned: string[] = [];
          for (const id of oldIds) {
            if (!newIds.has(id)) orphaned.push(id);
          }

          if (orphaned.length > 0) {
            setTimeout(() => onOrphanedComments(orphaned), 0);
          }

          return null;
        },
      }),
    ];
  },
});

// ---------------------------------------------------------------------------
// Re-hydration: apply marks from comment store when editor mounts/remounts
// ---------------------------------------------------------------------------

/** Resolve the position range for a single comment in the doc. */
function resolveCommentRange(
  comment: CommentForEditor,
  doc: DocLike,
  docSize: number,
  getTextData: () => { fullText: string; normalizedFullText: string; positions: number[] },
): { from: number; to: number } | null {
  // Try position-based first — verify text matches to guard against stale positions
  if (comment.from != null && comment.to != null && comment.from < comment.to) {
    if (comment.from >= 0 && comment.to <= docSize + 1) {
      const textAtRange = doc.textBetween(comment.from, comment.to, " ");
      if (normalizeForSearch(textAtRange) === normalizeForSearch(comment.selectedText)) {
        return { from: comment.from, to: comment.to };
      }
    }
  }

  // Fallback to text search
  const td = getTextData();
  return findCommentByTextSearch(comment, td.fullText, td.normalizedFullText, td.positions);
}

/**
 * Apply comment marks to the editor from the comment store.
 * Skips comments that are already marked in the doc.
 * Deferred via setTimeout to avoid flushSync-in-lifecycle errors.
 */
export function rehydrateCommentMarks(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  editor: any,
  comments: CommentForEditor[],
): void {
  if (!editor) return;

  // Defer to next tick — editor.view.dispatch triggers flushSync internally,
  // which crashes if called inside a React useEffect.
  setTimeout(() => {
    if (editor.isDestroyed) return;

    const { doc, schema } = editor.state;
    const markType = schema.marks.commentMark;
    if (!markType) return;

    const existingIds = collectCommentIds(doc, "commentMark");
    const wantedIds = new Set(comments.map((c) => c.id));
    let { tr } = editor.state;
    let changed = false;

    // Remove marks whose IDs are no longer in the comments list
    for (const id of existingIds) {
      if (!wantedIds.has(id)) {
        tr = tr.removeMark(0, doc.content.size, markType.create({ commentId: id }));
        changed = true;
      }
    }

    // Add marks for comments not yet in the editor
    const toApply = comments.filter((c) => !existingIds.has(c.id));
    if (toApply.length > 0) {
      let textData: { fullText: string; normalizedFullText: string; positions: number[] } | null =
        null;
      const getTextData = () => {
        if (!textData) {
          const { text, positions } = getTextWithPositions(doc);
          textData = { fullText: text, normalizedFullText: normalizeForSearch(text), positions };
        }
        return textData;
      };

      const docSize = doc.content.size;
      for (const comment of toApply) {
        const range = resolveCommentRange(comment, doc, docSize, getTextData);
        if (range) {
          tr = tr.addMark(range.from, range.to, markType.create({ commentId: comment.id }));
          changed = true;
        }
      }
    }

    if (changed) {
      editor.view.dispatch(tr);
    }
  }, 0);
}
