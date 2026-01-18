'use client';

import { openSessionInEditor } from '@/lib/http';
import { useRequest } from '@/lib/http/use-request';

type OpenEditorOptions = {
  filePath?: string;
  line?: number;
  column?: number;
  editorId?: string;
  editorType?: string;
};

export function useOpenSessionInEditor(sessionId?: string | null) {
  const request = useRequest(async (options?: OpenEditorOptions) => {
    if (!sessionId) {
      return null;
    }
    const response = await openSessionInEditor(
      sessionId,
      {
        editor_id: options?.editorId,
        editor_type: options?.editorType,
        file_path: options?.filePath,
        line: options?.line,
        column: options?.column,
      },
      { cache: 'no-store' }
    );
    if (response?.url) {
      window.open(response.url, '_blank', 'noopener,noreferrer');
    }
    return response ?? null;
  });

  return {
    open: (options?: OpenEditorOptions) => request.run(options),
    status: request.status,
    isLoading: request.isLoading,
  };
}
