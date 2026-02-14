import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export type EditorContext =
  | 'code-editor'
  | 'diff-viewer'
  | 'chat-code-block'
  | 'chat-diff'
  | 'plan-editor';

export type EditorProvider = 'monaco' | 'codemirror' | 'pierre-diffs' | 'tiptap';

const VALID_PROVIDERS: Record<EditorContext, EditorProvider[]> = {
  'code-editor': ['monaco', 'codemirror'],
  'diff-viewer': ['monaco', 'pierre-diffs'],
  'chat-code-block': ['monaco', 'codemirror'],
  'chat-diff': ['monaco', 'pierre-diffs'],
  'plan-editor': ['tiptap'],
};

const DEFAULT_PROVIDERS: Record<EditorContext, EditorProvider> = {
  'code-editor': 'monaco',
  'diff-viewer': 'monaco',
  'chat-code-block': 'monaco',
  'chat-diff': 'monaco',
  'plan-editor': 'tiptap',
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
    { name: 'kandev-editor-providers' }
  )
);
