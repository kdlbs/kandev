"use client";

import { openSessionInEditor } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { openFileInVscode } from "@/lib/api/domains/vscode-api";
import { useToast } from "@/components/toast-provider";

type OpenEditorOptions = {
  filePath?: string;
  line?: number;
  column?: number;
  editorId?: string;
  editorType?: string;
  worktreeId?: string;
  /** `filePath` points at a folder, so the embedded editor has nothing to goto. */
  isDirectory?: boolean;
};

/**
 * Parse an `internal://vscode?goto=file:line:col` sentinel URL.
 * Returns goto info or null if no file param.
 */
function parseInternalVscodeURL(url: string): { file: string; line: number; col: number } | null {
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

/**
 * Reveals the embedded editor panel and, for a file target, gotos it through
 * the backend Remote CLI. Folder targets have nothing to goto — the panel is
 * already rooted at the workspace.
 */
function openInternalVscode(sessionId: string, url: string, options?: OpenEditorOptions) {
  const goto_ = parseInternalVscodeURL(url);
  useDockviewStore.getState().openInternalVscode(null);
  if (goto_ && !options?.isDirectory) {
    openFileInVscode(sessionId, goto_.file, goto_.line, goto_.col);
  }
}

export function useOpenSessionInEditor(sessionId?: string | null) {
  const { toast } = useToast();
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
        worktree_id: options?.worktreeId,
      },
      { cache: "no-store" },
    );

    if (response?.url) {
      // Intercept internal VS Code URLs — handle in-app instead of opening a tab.
      if (response.url.startsWith("internal://vscode")) {
        openInternalVscode(sessionId, response.url, options);
        return response;
      }

      // Editor integrations may return registered custom schemes such as vscode://.
      window.open(response.url, "_blank", "noopener,noreferrer");
    }
    return response ?? null;
  });

  return {
    open: async (options?: OpenEditorOptions) => {
      try {
        return await request.run(options);
      } catch (error) {
        toast({
          title: "Failed to open editor",
          description: error instanceof Error ? error.message : "Request failed",
          variant: "error",
        });
        return null;
      }
    },
    status: request.status,
    isLoading: request.isLoading,
  };
}
