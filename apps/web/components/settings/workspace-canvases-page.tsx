"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  IconArchive,
  IconEdit,
  IconKey,
  IconRefresh,
  IconRestore,
  IconTrash,
} from "@tabler/icons-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import {
  archiveCanvas,
  canvasHref,
  listWorkspaceCanvases,
  removeCanvas,
  restoreCanvas,
  startCanvasEdit,
  type Canvas,
} from "@/lib/api/domains/canvas-api";
import { canvasErrorMessage } from "@/lib/api/domains/canvas-error-copy";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";
import { CanvasTaskCreateLauncher } from "@/components/canvas/canvas-task-create-launcher";
import { SettingsErrorText, SettingsPageHeader } from "./settings-typography";
import { CanvasReleaseDialog } from "./canvas-lifecycle-dialogs";

function canvasStatusLabel(status: string, t: (key: string) => string): string {
  const labels: Record<string, string> = {
    active: t("canvases:statusActive"),
    archived: t("canvases:statusArchived"),
    pending: t("canvases:statusPending"),
    disabled: t("canvases:statusDisabled"),
    error: t("canvases:statusError"),
  };
  return labels[status] ?? status;
}

function useWorkspaceCanvasState(workspaceId: string) {
  const { t } = useTranslation();
  const router = useRouter();
  const [canvases, setCanvases] = useState<Canvas[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [releaseCanvas, setReleaseCanvas] = useState<Canvas | null>(null);
  const [removeTarget, setRemoveTarget] = useState<Canvas | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const requestRef = useRef(0);
  const lifecycleRevision = useCanvasLifecycleRevision();

  const reload = useCallback(() => {
    const requestId = ++requestRef.current;
    setLoading(true);
    setError(null);
    listWorkspaceCanvases(workspaceId, { includeArchived: true })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setCanvases((response.canvases ?? []).filter((canvas) => canvas.status !== "removed"));
      })
      .catch((reason: unknown) => {
        if (requestRef.current === requestId) {
          setError(canvasErrorMessage(reason, t, "canvases:loadFailed"));
        }
      })
      .finally(() => {
        if (requestRef.current === requestId) setLoading(false);
      });
  }, [t, workspaceId]);

  useEffect(() => {
    reload();
    return () => {
      requestRef.current += 1;
    };
  }, [lifecycleRevision, reload]);

  const edit = async (canvas: Canvas) => {
    setBusyId(canvas.id);
    setError(null);
    try {
      const response = await startCanvasEdit(canvas.id);
      if (response.task_id) {
        const query = response.session_id
          ? `?sessionId=${encodeURIComponent(response.session_id)}`
          : "";
        router.push(`/t/${encodeURIComponent(response.task_id)}${query}`);
      }
    } catch (reason: unknown) {
      setError(canvasErrorMessage(reason, t, "canvases:actionFailed"));
    } finally {
      setBusyId(null);
    }
  };

  const changeStatus = async (canvas: Canvas, action: "archive" | "restore") => {
    setBusyId(canvas.id);
    setError(null);
    try {
      const next =
        action === "archive" ? await archiveCanvas(canvas.id) : await restoreCanvas(canvas.id);
      setCanvases((current) => current.map((item) => (item.id === next.id ? next : item)));
    } catch (reason: unknown) {
      setError(canvasErrorMessage(reason, t, "canvases:actionFailed"));
    } finally {
      setBusyId(null);
    }
  };

  const remove = async () => {
    if (!removeTarget) return;
    const target = removeTarget;
    setBusyId(target.id);
    setError(null);
    try {
      await removeCanvas(target.id);
      setCanvases((current) => current.filter((canvas) => canvas.id !== target.id));
      setRemoveTarget(null);
    } catch (reason: unknown) {
      setError(canvasErrorMessage(reason, t, "canvases:actionFailed"));
    } finally {
      setBusyId(null);
    }
  };

  return {
    canvases,
    loading,
    error,
    releaseCanvas,
    removeTarget,
    busyId,
    active: canvases.filter((canvas) => canvas.status !== "archived"),
    archived: canvases.filter((canvas) => canvas.status === "archived"),
    reload,
    edit,
    changeStatus,
    remove,
    setCanvases,
    setReleaseCanvas,
    setRemoveTarget,
  };
}

export function WorkspaceCanvasesPage({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const workspaceName = useAppStore(
    (state) => state.workspaces.items.find((workspace) => workspace.id === workspaceId)?.name,
  );
  const {
    canvases,
    loading,
    error,
    releaseCanvas,
    removeTarget,
    busyId,
    active,
    archived,
    reload,
    edit,
    changeStatus,
    remove,
    setCanvases,
    setReleaseCanvas,
    setRemoveTarget,
  } = useWorkspaceCanvasState(workspaceId);

  return (
    <div className="space-y-6" data-testid="workspace-canvases-page">
      <SettingsPageHeader
        title={t("canvases:workspaceCanvases")}
        description={t("canvases:workspaceCanvasesDescription", {
          workspace: workspaceName ?? t("common:workspace"),
        })}
        actions={
          <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row">
            <CanvasTaskCreateLauncher workspaceId={workspaceId} />
            <Button
              variant="outline"
              className="min-h-11 w-full cursor-pointer md:min-h-7 md:w-auto"
              onClick={reload}
              disabled={loading}
            >
              <IconRefresh className="mr-1.5 h-4 w-4" />
              {t("canvases:refresh")}
            </Button>
          </div>
        }
      />
      {error && <SettingsErrorText>{error}</SettingsErrorText>}
      {loading && <p role="status">{t("canvases:loadingCanvases")}</p>}
      {!loading && canvases.length === 0 && (
        <div className="rounded-lg border border-dashed p-6 text-sm text-muted-foreground">
          {t("canvases:noWorkspaceCanvases")}
        </div>
      )}
      {active.length > 0 && (
        <CanvasGroup
          title={t("canvases:activeCanvases")}
          canvases={active}
          busyId={busyId}
          onEdit={edit}
          onReleases={setReleaseCanvas}
          onArchive={(canvas) => void changeStatus(canvas, "archive")}
          onRestore={(canvas) => void changeStatus(canvas, "restore")}
          onRemove={setRemoveTarget}
        />
      )}
      {archived.length > 0 && (
        <CanvasGroup
          title={t("canvases:archivedCanvases")}
          canvases={archived}
          busyId={busyId}
          onEdit={edit}
          onReleases={setReleaseCanvas}
          onArchive={(canvas) => void changeStatus(canvas, "archive")}
          onRestore={(canvas) => void changeStatus(canvas, "restore")}
          onRemove={setRemoveTarget}
        />
      )}
      <CanvasReleaseDialog
        canvas={releaseCanvas}
        open={releaseCanvas !== null}
        onOpenChange={(open) => {
          if (!open) setReleaseCanvas(null);
        }}
        onChanged={(next) =>
          setCanvases((current) => current.map((item) => (item.id === next.id ? next : item)))
        }
      />
      <CanvasRemoveDialog
        target={removeTarget}
        busy={busyId === removeTarget?.id}
        onOpenChange={(open) => !open && setRemoveTarget(null)}
        onConfirm={remove}
      />
    </div>
  );
}

function CanvasGroup({
  title,
  canvases,
  busyId,
  onEdit,
  onReleases,
  onArchive,
  onRestore,
  onRemove,
}: {
  title: string;
  canvases: Canvas[];
  busyId: string | null;
  onEdit: (canvas: Canvas) => void;
  onReleases: (canvas: Canvas) => void;
  onArchive: (canvas: Canvas) => void;
  onRestore: (canvas: Canvas) => void;
  onRemove: (canvas: Canvas) => void;
}) {
  return (
    <section className="space-y-3">
      <h3 className="text-lg font-semibold">{title}</h3>
      <div className="space-y-3">
        {canvases.map((canvas) => (
          <CanvasRow
            key={canvas.id}
            canvas={canvas}
            busyId={busyId}
            onEdit={onEdit}
            onReleases={onReleases}
            onArchive={onArchive}
            onRestore={onRestore}
            onRemove={onRemove}
          />
        ))}
      </div>
    </section>
  );
}

function CanvasRemoveDialog({
  target,
  busy,
  onOpenChange,
  onConfirm,
}: {
  target: Canvas | null;
  busy: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={target !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("canvases:removeCanvas")}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("canvases:removeCanvasDescription", { title: target?.title ?? "" })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="min-h-11 cursor-pointer md:min-h-7">
            {t("common:cancel")}
          </AlertDialogCancel>
          <AlertDialogAction
            className="min-h-11 cursor-pointer md:min-h-7"
            disabled={busy}
            onClick={(event) => {
              event.preventDefault();
              void onConfirm();
            }}
          >
            {t("canvases:removeCanvas")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function CanvasRow({
  canvas,
  busyId,
  onEdit,
  onReleases,
  onArchive,
  onRestore,
  onRemove,
}: {
  canvas: Canvas;
  busyId: string | null;
  onEdit: (canvas: Canvas) => void;
  onReleases: (canvas: Canvas) => void;
  onArchive: (canvas: Canvas) => void;
  onRestore: (canvas: Canvas) => void;
  onRemove: (canvas: Canvas) => void;
}) {
  const { t } = useTranslation();
  const isArchived = canvas.status === "archived";
  return (
    <article className="rounded-lg border p-4" data-testid={`workspace-canvas-${canvas.id}`}>
      <div className="flex min-w-0 flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0">
          <h4 className="truncate font-semibold">{canvas.title}</h4>
          <p className="mt-1 text-xs text-muted-foreground">
            {canvasStatusLabel(canvas.status, t)}
            {canvas.active_release_id ? ` · ${canvas.active_release_id}` : ""}
          </p>
        </div>
        <div className="flex flex-wrap gap-2 md:justify-end">
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 cursor-pointer md:min-h-7"
            asChild
          >
            <Link href={canvasHref(canvas.id)}>{t("canvases:openCanvas")}</Link>
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 cursor-pointer md:min-h-7"
            disabled={
              busyId === canvas.id || canvas.status === "archived" || canvas.status === "disabled"
            }
            onClick={() => onEdit(canvas)}
          >
            <IconEdit className="mr-1.5 h-3.5 w-3.5" />
            {t("canvases:editCanvas")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="min-h-11 cursor-pointer md:min-h-7"
            onClick={() => onReleases(canvas)}
          >
            <IconKey className="mr-1.5 h-3.5 w-3.5" />
            {t("canvases:permissionsAndReleases")}
          </Button>
          <CanvasStatusAction
            canvas={canvas}
            busy={busyId === canvas.id}
            archived={isArchived}
            onArchive={onArchive}
            onRestore={onRestore}
          />
          <Button
            variant="destructive"
            size="sm"
            className="min-h-11 cursor-pointer md:min-h-7"
            disabled={busyId === canvas.id}
            onClick={() => onRemove(canvas)}
          >
            <IconTrash className="mr-1.5 h-3.5 w-3.5" />
            {t("canvases:removeCanvas")}
          </Button>
        </div>
      </div>
    </article>
  );
}

function CanvasStatusAction({
  canvas,
  busy,
  archived,
  onArchive,
  onRestore,
}: {
  canvas: Canvas;
  busy: boolean;
  archived: boolean;
  onArchive: (canvas: Canvas) => void;
  onRestore: (canvas: Canvas) => void;
}) {
  const { t } = useTranslation();
  return archived ? (
    <Button
      variant="outline"
      size="sm"
      className="min-h-11 cursor-pointer md:min-h-7"
      disabled={busy}
      onClick={() => onRestore(canvas)}
    >
      <IconRestore className="mr-1.5 h-3.5 w-3.5" />
      {t("canvases:restoreCanvas")}
    </Button>
  ) : (
    <Button
      variant="outline"
      size="sm"
      className="min-h-11 cursor-pointer md:min-h-7"
      disabled={busy}
      onClick={() => onArchive(canvas)}
    >
      <IconArchive className="mr-1.5 h-3.5 w-3.5" />
      {t("canvases:archiveCanvas")}
    </Button>
  );
}
