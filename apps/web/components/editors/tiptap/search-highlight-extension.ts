import { Extension } from "@tiptap/core";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import type { EditorState } from "@tiptap/pm/state";
import type { Node as PmNode } from "@tiptap/pm/model";

export type PlanSearchMatch = {
  from: number;
  to: number;
};

export type PlanSearchState = {
  query: string;
  matches: PlanSearchMatch[];
  current: number;
};

type SearchMeta =
  | { kind: "setQuery"; query: string }
  | { kind: "step"; delta: number }
  | { kind: "clear" };

export const planSearchPluginKey = new PluginKey<PlanSearchState>("planSearch");

declare module "@tiptap/core" {
  interface Commands<ReturnType> {
    planSearch: {
      setPlanSearchQuery: (query: string) => ReturnType;
      planSearchNext: () => ReturnType;
      planSearchPrev: () => ReturnType;
      clearPlanSearch: () => ReturnType;
    };
  }
}

function escapeForRegex(str: string): string {
  return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function findMatches(doc: PmNode, query: string): PlanSearchMatch[] {
  if (!query) return [];
  const matches: PlanSearchMatch[] = [];
  const re = new RegExp(escapeForRegex(query), "gi");
  doc.descendants((node, pos) => {
    if (!node.isText || !node.text) return;
    const text = node.text;
    let m: RegExpExecArray | null;
    re.lastIndex = 0;
    while ((m = re.exec(text)) !== null) {
      matches.push({ from: pos + m.index, to: pos + m.index + m[0].length });
      if (m.index === re.lastIndex) re.lastIndex++;
    }
  });
  return matches;
}

export const PlanSearchExtension = Extension.create({
  name: "planSearch",

  addProseMirrorPlugins() {
    return [
      new Plugin<PlanSearchState>({
        key: planSearchPluginKey,
        state: {
          init: (): PlanSearchState => ({ query: "", matches: [], current: 0 }),
          apply(tr, prev, _oldState, newState): PlanSearchState {
            const meta = tr.getMeta(planSearchPluginKey) as SearchMeta | undefined;
            let next = prev;
            if (meta) {
              if (meta.kind === "setQuery") {
                const matches = findMatches(newState.doc, meta.query);
                next = { query: meta.query, matches, current: matches.length ? 0 : 0 };
              } else if (meta.kind === "clear") {
                next = { query: "", matches: [], current: 0 };
              } else if (meta.kind === "step") {
                if (!prev.matches.length) next = prev;
                else {
                  const len = prev.matches.length;
                  const c = (prev.current + meta.delta + len) % len;
                  next = { ...prev, current: c };
                }
              }
            } else if (tr.docChanged && prev.query) {
              const matches = findMatches(newState.doc, prev.query);
              const boundedCurrent = Math.min(prev.current, Math.max(matches.length - 1, 0));
              next = { ...prev, matches, current: boundedCurrent };
            }
            return next;
          },
        },
        props: {
          decorations(state: EditorState): DecorationSet | null {
            const s = planSearchPluginKey.getState(state);
            if (!s || !s.matches.length) return null;
            const decos = s.matches.map((m, i) =>
              Decoration.inline(m.from, m.to, {
                class:
                  i === s.current
                    ? "search-highlight search-highlight-current"
                    : "search-highlight",
              }),
            );
            return DecorationSet.create(state.doc, decos);
          },
        },
      }),
    ];
  },

  addCommands() {
    return {
      setPlanSearchQuery:
        (query: string) =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            tr.setMeta(planSearchPluginKey, { kind: "setQuery", query } satisfies SearchMeta);
            dispatch(tr);
          }
          return true;
        },
      planSearchNext:
        () =>
        ({ state, tr, dispatch, view }) => {
          const s = planSearchPluginKey.getState(state);
          if (!s || !s.matches.length) return false;
          if (dispatch) {
            tr.setMeta(planSearchPluginKey, { kind: "step", delta: 1 } satisfies SearchMeta);
            dispatch(tr);
          }
          scrollToMatch(view, s.matches[(s.current + 1) % s.matches.length]);
          return true;
        },
      planSearchPrev:
        () =>
        ({ state, tr, dispatch, view }) => {
          const s = planSearchPluginKey.getState(state);
          if (!s || !s.matches.length) return false;
          if (dispatch) {
            tr.setMeta(planSearchPluginKey, { kind: "step", delta: -1 } satisfies SearchMeta);
            dispatch(tr);
          }
          const len = s.matches.length;
          scrollToMatch(view, s.matches[(s.current - 1 + len) % len]);
          return true;
        },
      clearPlanSearch:
        () =>
        ({ tr, dispatch }) => {
          if (dispatch) {
            tr.setMeta(planSearchPluginKey, { kind: "clear" } satisfies SearchMeta);
            dispatch(tr);
          }
          return true;
        },
    };
  },
});

function scrollToMatch(view: import("@tiptap/pm/view").EditorView, match: PlanSearchMatch): void {
  try {
    const coords = view.coordsAtPos(match.from);
    const el = view.dom as HTMLElement;
    const container = findScrollContainer(el);
    if (!container) return;
    const containerRect = container.getBoundingClientRect();
    if (coords.top < containerRect.top || coords.bottom > containerRect.bottom) {
      const offset = coords.top - containerRect.top - container.clientHeight / 3;
      container.scrollBy({ top: offset, behavior: "smooth" });
    }
  } catch {
    /* position may be transiently invalid */
  }
}

function findScrollContainer(el: HTMLElement | null): HTMLElement | null {
  let current: HTMLElement | null = el;
  while (current) {
    const overflowY = window.getComputedStyle(current).overflowY;
    if (overflowY === "auto" || overflowY === "scroll") return current;
    current = current.parentElement;
  }
  return null;
}

