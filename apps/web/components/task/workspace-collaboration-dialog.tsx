import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  useWorkspaceCollaboration,
  sameCollaborationFlags,
  type CollaborationRow,
} from "@/hooks/domains/workspace/use-workspace-collaboration";
import {
  COLLABORATION_ACTIONS,
  type CollaborationAction,
} from "@/lib/api/domains/workspace-collaboration-api";
import { settingsActionClassName } from "@/components/settings/settings-control";

const actionKeys = {
  read: "tasks:collaborationRead",
  report: "tasks:collaborationReport",
  prompt: "tasks:collaborationPrompt",
  control: "tasks:collaborationControl",
} as const;

function GrantRow({
  row,
  ready,
  edit,
  save,
}: {
  row: CollaborationRow;
  ready: boolean;
  edit: (id: string, action: CollaborationAction, checked: boolean) => void;
  save: (id: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const editable = ready && row.readable;
  return (
    <section
      className="min-w-0 space-y-2 rounded-md border p-3"
      aria-label={row.name}
      data-testid={`collaboration-target-${row.id}`}
    >
      <h3 className="break-words text-sm font-medium">{row.name}</h3>
      <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
        {COLLABORATION_ACTIONS.map((action) => (
          <label
            key={action}
            className="flex min-h-11 min-w-0 cursor-pointer items-center gap-2 rounded border px-2 text-sm md:min-h-7 [@media(pointer:coarse)]:min-h-11"
          >
            <Checkbox
              checked={row.draft[action]}
              disabled={!editable}
              aria-label={t(actionKeys[action])}
              onCheckedChange={(checked) => edit(row.id, action, checked === true)}
            />
            <span>{t(actionKeys[action])}</span>
          </label>
        ))}
      </div>
      {!row.readable && (
        <p className="text-xs text-muted-foreground">{t("tasks:collaborationTargetUnavailable")}</p>
      )}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs text-muted-foreground" role="status">
          {row.saving ? t("settings:saving") : null}
        </p>
        <Button
          type="button"
          variant="outline"
          className={settingsActionClassName("cursor-pointer")}
          disabled={!editable || row.saving || sameCollaborationFlags(row.saved, row.draft)}
          onClick={() => void save(row.id)}
        >
          {t("settings:saveChanges")}
        </Button>
      </div>
      {row.error && (
        <p className="text-sm text-destructive" role="alert">
          {t(
            row.error === "denied"
              ? "tasks:collaborationSaveDenied"
              : "tasks:collaborationSaveFailed",
          )}
        </p>
      )}
    </section>
  );
}

function GrantEditorBody({
  taskId,
  workspaceId,
  editor,
}: {
  taskId: string;
  workspaceId: string;
  editor: ReturnType<typeof useWorkspaceCollaboration>;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="min-h-0 min-w-0 flex-1 space-y-3 overflow-y-auto overscroll-contain p-4"
      data-testid="collaboration-grants"
      data-task-id={taskId}
      data-workspace-id={workspaceId}
    >
      <p className="text-xs text-muted-foreground">{t("tasks:collaborationNativePermissions")}</p>
      {editor.status === "loading" && (
        <p className="text-sm" role="status">
          {t("common:loading")}
        </p>
      )}
      {(editor.status === "denied" || editor.status === "failed") && (
        <div className="space-y-2">
          <p className="text-sm text-destructive" role="alert">
            {t(
              editor.status === "denied"
                ? "tasks:collaborationLoadDenied"
                : "tasks:collaborationLoadFailed",
            )}
          </p>
          <Button
            type="button"
            variant="outline"
            className={settingsActionClassName("cursor-pointer")}
            onClick={() => void editor.reload()}
          >
            {t("tasks:collaborationRetry")}
          </Button>
        </div>
      )}
      {editor.status === "ready" && editor.rows.length === 0 && (
        <p className="text-sm">{t("tasks:collaborationNoTargets")}</p>
      )}
      {editor.rows.map((row) => (
        <GrantRow
          key={row.id}
          row={row}
          ready={editor.status === "ready"}
          edit={editor.edit}
          save={editor.save}
        />
      ))}
    </div>
  );
}

export function WorkspaceCollaborationDialog({
  taskId,
  workspaceId,
  title,
  sourceName,
  open,
  onOpenChange,
}: {
  taskId: string;
  workspaceId: string;
  title: string;
  sourceName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const editor = useWorkspaceCollaboration(taskId, workspaceId, open);
  if (!editor.enabled) return null;
  const description = t("tasks:collaborationDescription", { task: title, workspace: sourceName });
  const body = <GrantEditorBody taskId={taskId} workspaceId={workspaceId} editor={editor} />;
  const footer = (
    <div className="flex shrink-0 justify-end border-t p-3 pb-[max(.75rem,env(safe-area-inset-bottom))]">
      <Button
        type="button"
        variant="outline"
        className={settingsActionClassName("cursor-pointer")}
        onClick={() => onOpenChange(false)}
      >
        {t("common:close")}
      </Button>
    </div>
  );
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="flex h-dvh max-h-dvh flex-col overflow-hidden data-[vaul-drawer-direction=bottom]:mt-0 data-[vaul-drawer-direction=bottom]:max-h-dvh"
          data-testid="collaboration-editor"
        >
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{t("tasks:collaborationTitle")}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          {body}
          <DrawerFooter className="shrink-0 p-0">{footer}</DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl"
        data-testid="collaboration-editor"
      >
        <DialogHeader className="shrink-0 p-4 text-left">
          <DialogTitle>{t("tasks:collaborationTitle")}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {body}
        {footer}
      </DialogContent>
    </Dialog>
  );
}
