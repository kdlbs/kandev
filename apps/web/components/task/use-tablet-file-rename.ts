import { useLayoutEffect, useRef, type Dispatch, type RefObject, type SetStateAction } from "react";
import type { OpenFileTab } from "@/lib/types/backend";
import {
  isWithin,
  retargetFile,
  treePath,
  type RenameAdapter,
} from "@/hooks/file-rename-continuity";
import { getFileTabKey, type FileEditorTab } from "./task-center-panel-file-tabs";

export function useTabletFileRename({
  adapterRef,
  session,
  tabs,
  setTabs,
  leftTab,
  setLeftTab,
}: {
  adapterRef: RefObject<RenameAdapter | null> | undefined;
  session: string | null;
  tabs: FileEditorTab[];
  setTabs: Dispatch<SetStateAction<OpenFileTab[]>>;
  leftTab: string;
  setLeftTab: Dispatch<SetStateAction<string>>;
}) {
  const live = useRef({ tabs, leftTab });
  useLayoutEffect(() => {
    live.current = { tabs, leftTab };
  }, [tabs, leftTab]);
  useLayoutEffect(() => {
    if (!adapterRef || !session) return;
    const adapter: RenameAdapter = {
      capture(oldPath, newPath) {
        const current = live.current.tabs;
        const affected = current.flatMap((file) => {
          const target = retargetFile(file, oldPath, newPath);
          return target ? [{ key: getFileTabKey(file), instance: file.instanceId, target }] : [];
        });
        const targetPaths = affected.map(({ target }) => treePath(target));
        const collision =
          targetPaths.some((p, index) => !p || targetPaths.indexOf(p) !== index) ||
          current.some((file) => {
            const full = treePath(file);
            return !full || (isWithin(full, newPath) && !retargetFile(file, oldPath, newPath));
          }) ||
          affected.some(({ instance }) => !instance);
        return {
          collision,
          publish() {
            if (adapterRef.current !== adapter) return true;
            const hasDestinationCollision = (files: FileEditorTab[]) =>
              files.some(
                (file) =>
                  !affected.some(
                    ({ key, instance }) =>
                      key === getFileTabKey(file) && instance === file.instanceId,
                  ) && targetPaths.includes(treePath(file)),
              );
            if (hasDestinationCollision(live.current.tabs)) return false;
            setTabs((previous) => {
              if (hasDestinationCollision(previous)) return previous;
              return previous.map((file: FileEditorTab) => {
                const item = affected.find(
                  ({ key, instance }) =>
                    key === getFileTabKey(file) && instance === file.instanceId,
                );
                return item ? { ...file, ...item.target } : file;
              });
            });
            setLeftTab((currentTab) => {
              const item = affected.find(
                ({ key, instance }) =>
                  currentTab === `file:${key}` &&
                  live.current.tabs.some(
                    (file) => getFileTabKey(file) === key && file.instanceId === instance,
                  ),
              );
              return item ? `file:${getFileTabKey(item.target)}` : currentTab;
            });
            return true;
          },
        };
      },
    };
    adapterRef.current = adapter;
    return () => {
      if (adapterRef.current === adapter) adapterRef.current = null;
    };
  }, [adapterRef, session, setTabs, setLeftTab]);
}
