"use client";

import { openSessionInEditor } from "@/lib/api";
import { useRequest } from "@/lib/http/use-request";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { openFileInVscode } from "@/lib/api/domains/vscode-api";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";

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
 * Decode the private path-only goto query once, with independent coordinates.
 * Returns goto info or null if no file param.
 */
function parseInternalVscodeURL(url: string): { file: string; line: number; col: number } | null {
  const params = new URL(url).searchParams;
  const file = params.get("goto");
  if (!file) return null;

  const line = parseInt(params.get("line") ?? "0", 10) || 0;
  const column = parseInt(params.get("column") ?? "0", 10) || 0;
  return {
    file,
    line: line > 0 ? line : 0,
    col: line > 0 && column > 0 ? column : 0,
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
          title: t("editors:failedToOpenEditor"),
          description: error instanceof Error ? error.message : t("common:requestFailed"),
          variant: "error",
        });
        return null;
      }
    },
    status: request.status,
    isLoading: request.isLoading,
  };
}
