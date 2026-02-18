/**
 * TipTap extension for comment highlight decorations.
 * Ported from the Milkdown ProseMirror plugin in markdown-editor.tsx.
 */

import { Extension } from '@tiptap/core';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';

export type CommentHighlight = {
  id: string;
  selectedText: string;
  comment: string;
};

const MIN_COMMENT_TEXT_LENGTH = 3;

const commentHighlightPluginKey = new PluginKey<{
  deco: DecorationSet;
  comments: CommentHighlight[];
}>('tiptap-comment-highlight');

// Helper to get plain text content and position mapping from a document
function getTextWithPositions(doc: { descendants: (fn: (node: { isText: boolean; text?: string; type: { name: string }; isBlock: boolean }, pos: number) => void) => void }): { text: string; positions: number[] } {
  let text = '';
  const positions: number[] = [];

  doc.descendants((node, pos) => {
    if (node.isText && node.text) {
      for (let i = 0; i < node.text.length; i++) {
        positions.push(pos + i);
        text += node.text[i];
      }
    } else if (node.type.name === 'hardBreak') {
      positions.push(-1);
      text += ' ';
    } else if (node.isBlock && text.length > 0 && !text.endsWith('\n')) {
      positions.push(-1);
      text += '\n';
    }
  });

  return { text, positions };
}

function normalizeForSearch(text: string): string {
  return text
    .replace(/\r\n/g, '\n')
    .replace(/[\t ]+/g, ' ')
    .replace(/\n+/g, '\n')
    .trim()
    .toLowerCase();
}

function createCommentDecorations(
  doc: Parameters<typeof DecorationSet.create>[0],
  comments: CommentHighlight[]
): DecorationSet {
  const decorations: Decoration[] = [];

  if (comments.length === 0) {
    return DecorationSet.empty;
  }

  const { text: fullText, positions } = getTextWithPositions(doc);
  const normalizedFullText = normalizeForSearch(fullText);

  for (const comment of comments) {
    const searchText = normalizeForSearch(comment.selectedText);
    if (!searchText || searchText.length < MIN_COMMENT_TEXT_LENGTH) continue;

    let normalizedIndex = normalizedFullText.indexOf(searchText);

    if (normalizedIndex === -1) {
      const firstLine = normalizeForSearch(comment.selectedText.split('\n')[0]);
      if (firstLine.length >= MIN_COMMENT_TEXT_LENGTH) {
        normalizedIndex = normalizedFullText.indexOf(firstLine);
        if (normalizedIndex !== -1) {
          addDecorationsForRange({ normalizedIndex, length: firstLine.length, comment, fullText, positions, decorations });
        }
      }
      continue;
    }

    addDecorationsForRange({ normalizedIndex, length: searchText.length, comment, fullText, positions, decorations });
  }

  return DecorationSet.create(doc, decorations);
}

interface AddDecorationsForRangeParams {
  normalizedIndex: number;
  length: number;
  comment: CommentHighlight;
  fullText: string;
  positions: number[];
  decorations: Decoration[];
}

/** Build a mapping from normalized character indices to original character indices. */
function buildNormToOrigMap(fullText: string, normalizedFullText: string): number[] {
  const normalizedChars = normalizedFullText.split('');
  const origChars = fullText.split('');
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

/** Push a highlight decoration if the range is valid. */
function pushDecoration(
  decorations: Decoration[],
  positions: number[],
  fromIdx: number,
  toIdx: number,
  comment: CommentHighlight,
) {
  const fromPos = positions[fromIdx];
  const toPos = positions[toIdx] + 1;
  if (fromPos >= 0 && toPos > fromPos) {
    decorations.push(
      Decoration.inline(fromPos, toPos, {
        class: 'comment-highlight',
        'data-comment-id': comment.id,
        title: comment.comment,
      })
    );
  }
}

function addDecorationsForRange({
  normalizedIndex,
  length,
  comment,
  fullText,
  positions,
  decorations,
}: AddDecorationsForRangeParams) {
  const normalizedFullText = normalizeForSearch(fullText);
  const normToOrig = buildNormToOrigMap(fullText, normalizedFullText);

  const startOrig = normToOrig[normalizedIndex] ?? 0;
  const endOrig = normToOrig[normalizedIndex + length - 1] ?? startOrig;

  let rangeStart = -1;

  for (let i = startOrig; i <= endOrig && i < positions.length; i++) {
    if (positions[i] === -1) {
      if (rangeStart !== -1) {
        pushDecoration(decorations, positions, rangeStart, i - 1, comment);
        rangeStart = -1;
      }
    } else if (rangeStart === -1) {
      rangeStart = i;
    }
  }

  if (rangeStart !== -1 && rangeStart < positions.length) {
    const lastValidIndex = Math.min(endOrig, positions.length - 1);
    pushDecoration(decorations, positions, rangeStart, lastValidIndex, comment);
  }
}

export const CommentHighlightExtension = Extension.create({
  name: 'commentHighlight',

  addOptions() {
    return {
      comments: [] as CommentHighlight[],
    };
  },

  addProseMirrorPlugins() {
    const initialComments = this.options.comments as CommentHighlight[];

    return [
      new Plugin({
        key: commentHighlightPluginKey,
        state: {
          init(_, state) {
            return {
              deco: createCommentDecorations(state.doc, initialComments),
              comments: initialComments,
            };
          },
          apply(tr, value, _, newState) {
            const newComments = tr.getMeta(commentHighlightPluginKey);
            if (newComments !== undefined) {
              return {
                deco: createCommentDecorations(newState.doc, newComments),
                comments: newComments,
              };
            }
            if (tr.docChanged) {
              return {
                deco: createCommentDecorations(newState.doc, value.comments),
                comments: value.comments,
              };
            }
            return value;
          },
        },
        props: {
          decorations(state) {
            const pluginState = commentHighlightPluginKey.getState(state);
            return pluginState?.deco ?? DecorationSet.empty;
          },
        },
      }),
    ];
  },
});

// Helper to update comments from outside
export function updateCommentHighlights(
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  editor: { view: { dispatch: (tr: any) => void; state: { tr: { setMeta: (key: PluginKey<any>, value: unknown) => any } } } },
  comments: CommentHighlight[]
) {
  const tr = editor.view.state.tr.setMeta(commentHighlightPluginKey, comments);
  editor.view.dispatch(tr);
}
