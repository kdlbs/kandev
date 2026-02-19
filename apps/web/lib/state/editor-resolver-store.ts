import { create } from "zustand";
import { persist } from "zustand/middleware";

export type EditorContext =
  | "code-editor"
  | "diff-viewer"
  | "chat-code-block"
  | "chat-diff"
  | "plan-editor";

export type EditorProvider = "monaco" | "codemirror" | "pierre-diffs" | "tiptap";

const MONACO: EditorProvider = "monaco";
const PIERRE_DIFFS: EditorProvider = "pierre-diffs";

const VALID_PROVIDERS: Record<EditorContext, EditorProvider[]> = {
  "code-editor": [MONACO, "codemirror"],
  "diff-viewer": [MONACO, PIERRE_DIFFS],
  "chat-code-block": [MONACO, "codemirror"],
  "chat-diff": [MONACO, PIERRE_DIFFS],
  "plan-editor": ["tiptap"],
};

const DEFAULT_PROVIDERS: Record<EditorContext, EditorProvider> = {
  "code-editor": MONACO,
  "diff-viewer": PIERRE_DIFFS,
  "chat-code-block": MONACO,
  "chat-diff": PIERRE_DIFFS,
  "plan-editor": "tiptap",
};

type EditorResolverStore = {
  providers: Record<EditorContext, EditorProvider>;
  setProvider: (context: EditorContext, provider: EditorProvider) => void;
  getProvider: (context: EditorContext) => EditorProvider;
  getValidProviders: (context: EditorContext) => EditorProvider[];
};

export const useEditorResolverStore = create<EditorResolverStore>()(
  persist(
    (set, get) => ({
      providers: { ...DEFAULT_PROVIDERS },
      setProvider: (context, provider) => {
        if (!VALID_PROVIDERS[context].includes(provider)) return;
        set((state) => ({
          providers: { ...state.providers, [context]: provider },
        }));
      },
      getProvider: (context) => get().providers[context],
      getValidProviders: (context) => VALID_PROVIDERS[context],
    }),
    {
      name: "kandev-editor-providers",
      version: 1,
      migrate: (persisted) => {
        const state = persisted as { providers: Record<EditorContext, EditorProvider> };
        return {
          ...state,
          providers: { ...state.providers, "diff-viewer": PIERRE_DIFFS, "chat-diff": PIERRE_DIFFS },
        };
      },
    },
  ),
);
