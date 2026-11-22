import { useEffect, type Dispatch, type RefObject, type SetStateAction } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import { setOpenFileTabs as saveOpenFileTabs } from "@/lib/local-storage";
import { getFilePreviewKind } from "@/lib/utils/file-types";
import type { OpenFileTab } from "@/lib/types/backend";
import type { RenameAdapter } from "@/hooks/file-rename-continuity";
import { useTabletFileRename } from "./use-tablet-file-rename";
import { useFileTabRestoration } from "./task-center-panel-restoration";

function usePersistOpenFileTabs(
  activeSessionId: string | null,
  openFileTabs: OpenFileTab[],
  restoring: RefObject<boolean>,
  restoredSessionId: string | null,
) {
  useEffect(() => {
    if (!activeSessionId || restoring.current || restoredSessionId !== activeSessionId) return;
    saveOpenFileTabs(
      activeSessionId,
      openFileTabs.map(({ path, name, repo, renderedPreview }) => ({
        path,
        name,
        repo,
        ...(getFilePreviewKind(path) === "markdown" && renderedPreview ? { renderedPreview } : {}),
      })),
    );
  }, [activeSessionId, openFileTabs, restoring, restoredSessionId]);
}

export function useTabletFileState(options: {
  adapter: RefObject<RenameAdapter | null> | undefined;
  session: string | null;
  tabs: OpenFileTab[];
  tabsRef: RefObject<OpenFileTab[]>;
  setTabs: Dispatch<SetStateAction<OpenFileTab[]>>;
  selected: string;
  setSelected: Dispatch<SetStateAction<string>>;
}) {
  const { adapter, session, tabs, tabsRef, setTabs, selected, setSelected } = options;
  const store = useAppStoreApi();
  useTabletFileRename({
    adapterRef: adapter,
    session,
    tabs,
    setTabs,
    leftTab: selected,
    setLeftTab: setSelected,
  });
  const { restorationInProgressRef, restoredSessionId, retireFile } = useFileTabRestoration({
    store,
    activeSessionId: session,
    leftTab: selected,
    setLeftTab: setSelected,
    setOpenFileTabs: setTabs,
    openFileTabs: tabs,
    currentTabs: tabsRef,
  });
  usePersistOpenFileTabs(session, tabs, restorationInProgressRef, restoredSessionId);
  return { retireFile };
}
