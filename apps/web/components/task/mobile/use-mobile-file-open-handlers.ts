import { useCallback, type MutableRefObject } from "react";
import { useToast } from "@/components/toast-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import { isMarkdownFile } from "@/lib/utils/file-types";
import type { OpenFileTab } from "@/lib/types/backend";
import type { MarkdownFileMode } from "@/lib/types/workspace-files";
import type { MobileSessionPanel } from "@/lib/state/slices/ui/types";
import { defaultMarkdownFileMode } from "../markdown-file-mode";
import { fetchAndOpenFile } from "../file-browser-hooks";

type Options = {
  effectiveSessionId: string | null;
  selectedFileRef: MutableRefObject<OpenFileTab | null>;
  handlePanelChange: (panel: MobileSessionPanel) => void;
  requestFileOpen: (file: OpenFileTab, mode?: MarkdownFileMode) => void;
  latestRequestIdRef: MutableRefObject<number>;
  openFileAbortRef: MutableRefObject<AbortController | null>;
};

export function useMobileFileOpenHandlers({
  effectiveSessionId,
  selectedFileRef,
  handlePanelChange,
  requestFileOpen,
  latestRequestIdRef,
  openFileAbortRef,
}: Options) {
  const { toast } = useToast();
  const openFileWithIntent = useCallback(
    (path: string, repo: string | undefined, preview: boolean | "walkthrough") => {
      if (!effectiveSessionId) return;
      const current = selectedFileRef.current;
      if (
        preview === "walkthrough" &&
        current?.path === path &&
        (current.repo ?? "") === (repo ?? "")
      ) {
        handlePanelChange("files");
        return;
      }
      const requestId = (latestRequestIdRef.current += 1);
      openFileAbortRef.current?.abort();
      const controller = new AbortController();
      openFileAbortRef.current = controller;
      void Promise.resolve(
        fetchAndOpenFile(
          effectiveSessionId,
          path,
          (file) => {
            if (requestId !== latestRequestIdRef.current || controller.signal.aborted) return;
            let markdownMode: MarkdownFileMode | undefined;
            if (isMarkdownFile(file.path)) {
              const retained = useDockviewStore
                .getState()
                .fileDrafts.get(effectiveSessionId)
                ?.get(buildRepoScopedItemId(file.path, repo));
              if (preview === "walkthrough")
                markdownMode = retained?.markdownMode ?? defaultMarkdownFileMode(file.path);
              else markdownMode = preview ? "preview" : "source";
            }
            requestFileOpen(file, markdownMode);
          },
          toast,
          { repo, signal: controller.signal },
        ),
      ).finally(() => {
        if (openFileAbortRef.current === controller) {
          openFileAbortRef.current = null;
        }
      });
    },
    [
      effectiveSessionId,
      handlePanelChange,
      latestRequestIdRef,
      requestFileOpen,
      openFileAbortRef,
      toast,
    ],
  );

  const handleOpenFileFromChat = useCallback(
    (path: string, repo?: string, preview = false) => openFileWithIntent(path, repo, preview),
    [openFileWithIntent],
  );
  const handleOpenWalkthroughFile = useCallback(
    (path: string, repo?: string) => openFileWithIntent(path, repo, "walkthrough"),
    [openFileWithIntent],
  );

  return { handleOpenFileFromChat, handleOpenWalkthroughFile };
}
