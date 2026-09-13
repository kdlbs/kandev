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
import {
  confirmCanvasPromotion,
  requestCanvasPromotion,
  type Canvas,
  type CanvasPromotionPreview,
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

function buildPromotionMetadataRows(
  canvas: Canvas | null,
  preview: CanvasPromotionPreview,
  t: (key: string) => string,
): PromotionMetadataRow[] {
  const sourceTaskID =
    preview.source_task_id ?? preview.origin_task_id ?? canvas?.origin_task_id ?? canvas?.task_id;
  const sourceTask = sourceTaskID ? canvasSourceLabel(preview.source_task_title, t) : undefined;
  const sourceSessionID = preview.source_session_id ?? canvas?.created_by_session_id;
  const sourceSession = sourceSessionID
    ? canvasSourceLabel(preview.source_session_name, t)
    : undefined;
  const row = (
    value: string | undefined,
    labelKey: string,
    testId: string,
  ): PromotionMetadataRow | null => (value ? { label: t(labelKey), value, testId } : null);

  return [
    row(
      preview.current_scope ?? canvas?.scope_kind,
      "canvases:promotionSourceScope",
      "canvas-promotion-source-scope",
    ),
    row(
      preview.source_actor_kind ? canvasSourceActorLabel(preview.source_actor_kind, t) : undefined,
      "canvases:promotionSourceActor",
      "canvas-promotion-source-actor",
    ),
    row(sourceTask, "canvases:promotionSourceTask", "canvas-promotion-source-task"),
    row(sourceSession, "canvases:promotionSourceSession", "canvas-promotion-source-session"),
    row(preview.target_scope, "canvases:promotionTargetScope", "canvas-promotion-target-scope"),
    row(preview.placement, "canvases:promotionPlacement", "canvas-promotion-placement"),
  ].filter((item): item is PromotionMetadataRow => item !== null);
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
