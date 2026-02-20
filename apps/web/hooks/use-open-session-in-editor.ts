"use client";

import { openSessionInEditor } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { openFileInVscode } from "@/lib/api/domains/vscode-api";

type OpenEditorOptions = {
  filePath?: string;
  line?: number;
  column?: number;
  editorId?: string;
  editorType?: string;
};

/**
 * Parse an `internal://vscode?goto=file:line:col` sentinel URL.
 * Returns goto info or null if no file param.
 */
function parseInternalVscodeURL(
  url: string,
): { file: string; line: number; col: number } | null {
  const qIdx = url.indexOf("?goto=");
  if (qIdx === -1) return null;

  const goto_ = url.slice(qIdx + 6);
  if (!goto_) return null;

  // Format: file:line:col or file:line or file
  const parts = goto_.split(":");
  return {
    file: parts[0],
    line: parseInt(parts[1] ?? "0", 10) || 0,
    col: parseInt(parts[2] ?? "0", 10) || 0,
  };
}

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
      { cache: "no-store" },
    );

    if (response?.url) {
      // Intercept internal VS Code URLs — handle in-app instead of opening a tab.
      if (response.url.startsWith("internal://vscode")) {
        const goto_ = parseInternalVscodeURL(response.url);
        useDockviewStore.getState().openInternalVscode(null);

        // Open the file via the backend Remote CLI if goto params are provided
        if (goto_ && sessionId) {
          openFileInVscode(sessionId, goto_.file, goto_.line, goto_.col);
        }
        return response;
      }

      window.open(response.url, "_blank", "noopener,noreferrer");
    }
    return response ?? null;
  });

  return {
    open: (options?: OpenEditorOptions) => request.run(options),
    status: request.status,
    isLoading: request.isLoading,
  };
}
