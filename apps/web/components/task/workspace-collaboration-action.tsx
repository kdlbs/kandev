import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { useWorkspaceCollaborationMenu } from "@/hooks/use-workspace-collaboration-menu";
import { WorkspaceCollaborationDialog } from "./workspace-collaboration-dialog";

/** A conversation embedded without the task top bar shares its grant editor. */
export function WorkspaceCollaborationAction({
  taskId,
  title,
  workspace,
  workspaceId,
  mobile = false,
}: {
  taskId: string;
  title?: string;
  workspace?: { id: string; name: string; scopes?: string[] };
  workspaceId?: string;
  mobile?: boolean;
}) {
  const { t } = useTranslation();
  const sourceWorkspaceId = workspace?.id ?? workspaceId ?? null;
  const menu = useWorkspaceCollaborationMenu(taskId, sourceWorkspaceId, workspace);
  if (!menu.entries.length || !menu.sourceId) return null;
  return (
    <>
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName(
          mobile ? "min-h-11 w-full justify-start cursor-pointer" : "cursor-pointer",
        )}
        data-testid="conversation-workspace-collaboration"
        onClick={menu.show}
      >
        {t("tasks:collaborationTitle")}
      </Button>
      {menu.open && (
        <WorkspaceCollaborationDialog
          key={`${taskId}:${menu.sourceId}`}
          taskId={taskId}
          workspaceId={menu.sourceId}
          title={title ?? t("task:taskDetails")}
          sourceName={menu.sourceName}
          open
          onOpenChange={(next) => {
            if (!next) menu.close();
          }}
        />
      )}
    </>
  );
}
