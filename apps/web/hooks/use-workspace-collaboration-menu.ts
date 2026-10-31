import { useFeature } from "@/hooks/domains/features/use-feature";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useEmbeddedTaskWorkspace } from "@/components/task/task-workspace-context";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import type { KanbanCardMenuEntry } from "@/components/kanban-card-menu-items";

type WorkspaceReach = { id: string; name: string; scopes?: string[] };

export function useWorkspaceCollaborationMenu(
  taskId: string | null,
  workspaceId: string | null,
  workspace?: WorkspaceReach,
) {
  const { t } = useTranslation();
  const embedded = useEmbeddedTaskWorkspace();
  const sourceId = embedded?.taskId === taskId ? embedded.workspaceId : workspaceId;
  const enabled = useFeature("workspaceCollaboration");
  const stored = useAppStore((s) => s.workspaces.items.find((w) => w.id === sourceId));
  const source = workspace?.id === sourceId ? workspace : stored;
  const allowed = enabled && hasScope(source?.scopes, SCOPE.workspaceManage);
  const key = JSON.stringify([taskId, sourceId]);
  const [openedKey, setOpenedKey] = useState<string | null>(null);
  const close = useCallback(() => setOpenedKey(null), []);
  const show = useCallback(() => setOpenedKey(key), [key]);
  useEffect(() => {
    setOpenedKey(null);
  }, [key, allowed]);
  const entries: KanbanCardMenuEntry[] =
    allowed && taskId && sourceId
      ? [
          {
            kind: "item",
            key: "workspace-collaboration",
            label: t("tasks:collaborationTitle"),
            testId: "task-workspace-collaboration",
            onSelect: show,
          },
        ]
      : [];
  return {
    entries,
    taskId,
    sourceId,
    sourceName: source?.name ?? sourceId ?? "",
    open: allowed && openedKey === key,
    close,
    show,
  };
}
