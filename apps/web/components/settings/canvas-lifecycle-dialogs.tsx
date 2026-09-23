"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  enableCanvasWorkspaceData,
  confirmCanvasPromotion,
  requestCanvasWorkspaceData,
  requestCanvasPromotion,
  type Canvas,
  type CanvasPromotionPreview,
  type CanvasWorkspaceDataPreview,
} from "@/lib/api/domains/canvas-api";
import { canvasErrorMessage } from "@/lib/api/domains/canvas-error-copy";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import {
  buildCanvasPermissionGroups,
  canvasSourceActorLabel,
  canvasSourceLabel,
} from "@/lib/canvas-permission-copy";
import { CanvasPermissionSummary, hasUnsupportedPermissions } from "./canvas-permission-summary";

export { CanvasReleaseDialog } from "./canvas-release-review";

const CANVAS_ACTION_FAILED_KEY = "canvases:actionFailed";
const canvasActionClassName = controlSizingClassName("standard", "cursor-pointer");
const promotionScopeKeys = {
  workspaceData: "canvases:workspaceDataScope",
  taskData: "canvases:taskDataScope",
  workspacePlacement: "canvases:workspacePlacementScope",
  taskPlacement: "canvases:taskPlacementScope",
} as const;

function useCanvasPromotion(
  canvas: Canvas | null,
  open: boolean,
  onOpenChange: (open: boolean) => void,
  onCompleted?: (canvas: Canvas) => void,
) {
  const { t } = useTranslation();
  const lifecycleRevision = useCanvasLifecycleRevision();
  const [preview, setPreview] = useState<CanvasPromotionPreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open || !canvas) return;
    let cancelled = false;
    setLoading(true);
    setPreview(null);
    setError(null);
    requestCanvasPromotion(canvas.id)
      .then((value) => {
        if (!cancelled) setPreview(value);
      })
      .catch((reason: unknown) => {
        if (!cancelled) setError(canvasErrorMessage(reason, t, CANVAS_ACTION_FAILED_KEY));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [canvas, lifecycleRevision, open, t]);

  const permissionGroups = buildCanvasPermissionGroups(preview?.permissions, undefined, t);
  const unsupportedPermissions = hasUnsupportedPermissions(permissionGroups);
  const confirm = useCallback(async () => {
    if (
      !canvas ||
      !preview?.active_release_id ||
      !preview.permission_digest ||
      preview.grant_generation === undefined ||
      unsupportedPermissions
    ) {
      return;
    }
    setConfirming(true);
    setError(null);
    try {
      const promoted = await confirmCanvasPromotion(canvas.id, {
        expected_release_id: preview.active_release_id,
        expected_permission_digest: preview.permission_digest,
        expected_grant_generation: preview.grant_generation,
      });
      onCompleted?.(promoted);
      onOpenChange(false);
    } catch (reason: unknown) {
      setError(canvasErrorMessage(reason, t, CANVAS_ACTION_FAILED_KEY));
    } finally {
      setConfirming(false);
    }
  }, [canvas, onCompleted, onOpenChange, preview, t, unsupportedPermissions]);

  return { preview, loading, confirming, error, confirm, permissionGroups, unsupportedPermissions };
}

type PromotionMetadataRow = {
  label: string;
  value: string;
  testId: string;
};

function promotionScopeLabel(
  scope: string | undefined,
  workspaceKey: string,
  taskKey: string,
  t: (key: string) => string,
) {
  if (scope === "workspace") return t(workspaceKey);
  if (scope === "task") return t(taskKey);
  return scope;
}

function buildPromotionMetadataRows(
  canvas: Canvas | null,
  preview: CanvasPromotionPreview,
  t: (key: string) => string,
): PromotionMetadataRow[] {
  const placementRow = optionalPromotionRow(
    preview.placement,
    "canvases:promotionPlacement",
    "canvas-promotion-placement",
    t,
  );
  return [
    ...buildPromotionPlacementRows(canvas, preview, t),
    ...buildPromotionSourceRows(canvas, preview, t),
    ...buildPromotionDataScopeRows(canvas, preview, t),
    ...(placementRow ? [placementRow] : []),
  ];
}

function buildPromotionPlacementRows(
  canvas: Canvas | null,
  preview: CanvasPromotionPreview,
  t: (key: string) => string,
): PromotionMetadataRow[] {
  return [
    {
      label: t("canvases:promotionSourceScope"),
      value:
        promotionScopeLabel(
          preview.current_scope ?? canvas?.scope_kind,
          promotionScopeKeys.workspacePlacement,
          promotionScopeKeys.taskPlacement,
          t,
        ) ?? "",
      testId: "canvas-promotion-source-scope",
    },
    {
      label: t("canvases:promotionTargetScope"),
      value:
        promotionScopeLabel(
          preview.target_scope,
          promotionScopeKeys.workspacePlacement,
          promotionScopeKeys.taskPlacement,
          t,
        ) ?? "",
      testId: "canvas-promotion-target-scope",
    },
  ].filter((row) => row.value !== "");
}

function buildPromotionSourceRows(
  canvas: Canvas | null,
  preview: CanvasPromotionPreview,
  t: (key: string) => string,
): PromotionMetadataRow[] {
  const taskID =
    preview.source_task_id ?? preview.origin_task_id ?? canvas?.origin_task_id ?? canvas?.task_id;
  const sessionID = preview.source_session_id ?? canvas?.created_by_session_id;
  const actor = preview.source_actor_kind
    ? canvasSourceActorLabel(preview.source_actor_kind, t)
    : undefined;
  const task = taskID ? canvasSourceLabel(preview.source_task_title, t) : undefined;
  const session = sessionID ? canvasSourceLabel(preview.source_session_name, t) : undefined;
  return [
    optionalPromotionRow(
      actor,
      "canvases:promotionSourceActor",
      "canvas-promotion-source-actor",
      t,
    ),
    optionalPromotionRow(task, "canvases:promotionSourceTask", "canvas-promotion-source-task", t),
    optionalPromotionRow(
      session,
      "canvases:promotionSourceSession",
      "canvas-promotion-source-session",
      t,
    ),
  ].filter((row): row is PromotionMetadataRow => row !== null);
}

function buildPromotionDataScopeRows(
  canvas: Canvas | null,
  preview: CanvasPromotionPreview,
  t: (key: string) => string,
): PromotionMetadataRow[] {
  const currentScope =
    preview.current_data_scope_kind ?? canvas?.data_scope_kind ?? canvas?.scope_kind;
  return [
    {
      label: t("canvases:promotionCurrentDataScope"),
      value:
        promotionScopeLabel(
          currentScope,
          promotionScopeKeys.workspaceData,
          promotionScopeKeys.taskData,
          t,
        ) ?? "",
      testId: "canvas-promotion-current-data-scope",
    },
    {
      label: t("canvases:promotionTargetDataScope"),
      value:
        promotionScopeLabel(
          preview.target_data_scope_kind,
          promotionScopeKeys.workspaceData,
          promotionScopeKeys.taskData,
          t,
        ) ?? "",
      testId: "canvas-promotion-target-data-scope",
    },
  ].filter((row) => row.value !== "");
}

function optionalPromotionRow(
  value: string | undefined,
  labelKey: string,
  testId: string,
  t: (key: string) => string,
): PromotionMetadataRow | null {
  return value ? { label: t(labelKey), value, testId } : null;
}

function PromotionMetadata({
  canvas,
  preview,
}: {
  canvas: Canvas | null;
  preview: CanvasPromotionPreview;
}) {
  const { t } = useTranslation();
  const rows = buildPromotionMetadataRows(canvas, preview, t);

  if (rows.length === 0) return null;
  return (
    <dl className="grid gap-2 rounded-md bg-muted/40 p-3">
      {rows.map((row) => (
        <div key={row.testId} className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
          <dt className="text-muted-foreground">{row.label}</dt>
          <dd className="min-w-0 break-words" data-testid={row.testId}>
            {row.value}
          </dd>
        </div>
      ))}
    </dl>
  );
}

export function CanvasPromotionDialog({
  canvas,
  open,
  onOpenChange,
  onCompleted,
}: {
  canvas: Canvas | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCompleted?: (canvas: Canvas) => void;
}) {
  const { t } = useTranslation();
  const { preview, loading, confirming, error, confirm, permissionGroups, unsupportedPermissions } =
    useCanvasPromotion(canvas, open, onOpenChange, onCompleted);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="canvas-promotion-dialog" className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("canvases:promoteCanvas")}</DialogTitle>
          <DialogDescription>
            {t("canvases:promoteCanvasDescription", { title: canvas?.title ?? "" })}
          </DialogDescription>
        </DialogHeader>
        {loading && <p role="status">{t("canvases:loadingPermissions")}</p>}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {preview && (
          <div className="space-y-3 text-sm">
            <p>{t("canvases:promotionScopeChange")}</p>
            <p>
              {preview.current_data_scope_kind === preview.target_data_scope_kind
                ? t("canvases:promotionDataAccessPreserved")
                : t("canvases:promotionDataAccessChanges")}
            </p>
            <PromotionMetadata canvas={canvas} preview={preview} />
            {permissionGroups.length > 0 ? (
              <div className="max-h-48 space-y-3 overflow-y-auto rounded-md border p-3">
                <CanvasPermissionSummary permissions={preview.permissions} />
              </div>
            ) : (
              <p className="text-muted-foreground">{t("canvases:noAdditionalPermissions")}</p>
            )}
          </div>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            className={canvasActionClassName}
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            className={canvasActionClassName}
            disabled={
              !preview ||
              !preview.active_release_id ||
              !preview.permission_digest ||
              preview.grant_generation === undefined ||
              unsupportedPermissions ||
              confirming
            }
            onClick={() => void confirm()}
          >
            {confirming ? t("canvases:promotingCanvas") : t("canvases:confirmPromotion")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function useCanvasWorkspaceDataReview(
  canvas: Canvas | null,
  open: boolean,
  onOpenChange: (open: boolean) => void,
  onCompleted?: (canvas: Canvas) => void,
) {
  const { t } = useTranslation();
  const lifecycleRevision = useCanvasLifecycleRevision();
  const [preview, setPreview] = useState<CanvasWorkspaceDataPreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const canvasId = canvas?.id;

  useEffect(() => {
    if (!open || !canvasId) return;
    let cancelled = false;
    setLoading(true);
    setPreview(null);
    setError(null);
    requestCanvasWorkspaceData(canvasId)
      .then((value) => {
        if (!cancelled) setPreview(value);
      })
      .catch((reason: unknown) => {
        if (!cancelled) setError(canvasErrorMessage(reason, t, CANVAS_ACTION_FAILED_KEY));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [canvasId, lifecycleRevision, open, t]);

  const permissionGroups = buildCanvasPermissionGroups(preview?.permissions, undefined, t);
  const unsupportedPermissions = hasUnsupportedPermissions(permissionGroups);
  const confirm = useCallback(async () => {
    if (
      !canvas ||
      !preview?.active_release_id ||
      !preview.permission_digest ||
      preview.grant_generation === undefined ||
      unsupportedPermissions
    ) {
      return;
    }
    setConfirming(true);
    setError(null);
    try {
      const updated = await enableCanvasWorkspaceData(canvas.id, {
        expected_release_id: preview.active_release_id,
        expected_permission_digest: preview.permission_digest,
        expected_grant_generation: preview.grant_generation,
      });
      onCompleted?.(updated);
      onOpenChange(false);
    } catch (reason: unknown) {
      setError(canvasErrorMessage(reason, t, CANVAS_ACTION_FAILED_KEY));
    } finally {
      setConfirming(false);
    }
  }, [canvas, onCompleted, onOpenChange, preview, t, unsupportedPermissions]);

  return { preview, loading, confirming, error, confirm, unsupportedPermissions };
}

function WorkspaceDataReviewDetails({
  preview,
  loading,
  error,
  t,
}: {
  preview: CanvasWorkspaceDataPreview | null;
  loading: boolean;
  error: string | null;
  t: (key: string) => string;
}) {
  return (
    <div
      className="min-h-0 flex-1 space-y-3 overflow-y-auto overscroll-contain p-4 sm:p-6"
      data-testid="canvas-workspace-data-review-scroll"
    >
      {loading && <p role="status">{t("canvases:loadingPermissions")}</p>}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {preview && (
        <>
          <p className="text-sm">{t("canvases:workspaceDataReviewScopeChange")}</p>
          <dl className="grid gap-2 rounded-md bg-muted/40 p-3 text-sm">
            <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
              <dt className="text-muted-foreground">{t("canvases:activeRelease")}</dt>
              <dd className="min-w-0 break-all" data-testid="canvas-workspace-data-release">
                {preview.active_release_id}
              </dd>
            </div>
            <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
              <dt className="text-muted-foreground">{t("canvases:promotionCurrentDataScope")}</dt>
              <dd data-testid="canvas-workspace-data-current-scope">
                {t(
                  preview.current_data_scope_kind === "workspace"
                    ? "canvases:workspaceDataScope"
                    : "canvases:taskDataScope",
                )}
              </dd>
            </div>
            <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-3">
              <dt className="text-muted-foreground">{t("canvases:promotionTargetDataScope")}</dt>
              <dd data-testid="canvas-workspace-data-target-scope">
                {t(
                  preview.target_data_scope_kind === "task"
                    ? "canvases:taskDataScope"
                    : "canvases:workspaceDataScope",
                )}
              </dd>
            </div>
          </dl>
          <CanvasPermissionSummary permissions={preview.permissions} />
        </>
      )}
    </div>
  );
}

function WorkspaceDataReviewFooter({
  isMobile,
  disabled,
  confirming,
  onCancel,
  onConfirm,
  t,
}: {
  isMobile: boolean;
  disabled: boolean;
  confirming: boolean;
  onCancel: () => void;
  onConfirm: () => void;
  t: (key: string) => string;
}) {
  return (
    <DialogFooter
      className={
        isMobile
          ? "shrink-0 grid grid-cols-2 border-t border-border/70 p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
          : "shrink-0 flex-row border-t border-border/70 p-4 sm:p-6"
      }
    >
      <Button variant="outline" className={canvasActionClassName} onClick={onCancel}>
        {t("common:cancel")}
      </Button>
      <Button
        className={`${canvasActionClassName}${isMobile ? " min-h-12" : ""}`}
        disabled={disabled}
        onClick={onConfirm}
      >
        {confirming ? t("canvases:enablingWorkspaceData") : t("canvases:enableWorkspaceData")}
      </Button>
    </DialogFooter>
  );
}

export function CanvasWorkspaceDataDialog({
  canvas,
  open,
  onOpenChange,
  onCompleted,
}: {
  canvas: Canvas | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCompleted?: (canvas: Canvas) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const { preview, loading, confirming, error, confirm, unsupportedPermissions } =
    useCanvasWorkspaceDataReview(canvas, open, onOpenChange, onCompleted);
  const surfaceClassName = isMobile
    ? "!left-0 !top-0 !h-dvh !max-h-dvh !w-screen !max-w-none !translate-x-0 !translate-y-0 flex flex-col gap-0 overflow-hidden rounded-none p-0 [padding-top:max(1rem,env(safe-area-inset-top))]"
    : "flex max-h-[min(90dvh,48rem)] w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-lg";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="canvas-workspace-data-dialog"
        className={surfaceClassName}
        showCloseButton={false}
      >
        <DialogHeader className="shrink-0 border-b border-border/70 p-4 text-left sm:p-6">
          <DialogTitle>{t("canvases:enableWorkspaceData")}</DialogTitle>
          <DialogDescription>
            {t("canvases:workspaceDataReviewDescription", { title: canvas?.title ?? "" })}
          </DialogDescription>
        </DialogHeader>
        <WorkspaceDataReviewDetails preview={preview} loading={loading} error={error} t={t} />
        <WorkspaceDataReviewFooter
          isMobile={isMobile}
          disabled={
            !preview ||
            !preview.active_release_id ||
            !preview.permission_digest ||
            preview.grant_generation === undefined ||
            unsupportedPermissions ||
            confirming
          }
          confirming={confirming}
          onCancel={() => onOpenChange(false)}
          onConfirm={() => void confirm()}
          t={t}
        />
      </DialogContent>
    </Dialog>
  );
}
