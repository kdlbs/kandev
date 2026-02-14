import { useEditorResolverStore, type EditorContext } from '@/lib/state/editor-resolver-store';

export function useEditorProvider(context: EditorContext) {
  return useEditorResolverStore((s) => s.providers[context]);
}
