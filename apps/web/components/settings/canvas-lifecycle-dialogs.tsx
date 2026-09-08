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
  approveCanvasRelease,
  confirmCanvasPromotion,
  listCanvasReleases,
  rejectCanvasRelease,
  requestCanvasPromotion,
  rollbackCanvas,
  type Canvas,
  type CanvasPromotionPreview,
  type CanvasRelease,
} from "@/lib/api/domains/canvas-api";
import { canvasErrorCodeMessage, canvasErrorMessage } from "@/lib/api/domains/canvas-error-copy";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";

const CANVAS_ACTION_FAILED_KEY = "canvases:actionFailed";

function releaseStatusLabel(status: string, t: (key: string) => string): string {
  const labels: Record<string, string> = {
    valid: t("canvases:statusActive"),
    pending_permission: t("canvases:statusPending"),
    invalid: t("canvases:invalidRelease"),
    unavailable: t("canvases:unavailable"),
  };
  return labels[status] ?? status;
}

type PermissionGroup = {
  id: string;
  label: string;
  values: string[];
};

function buildPermissionGroups(
  permissions: CanvasPromotionPreview["permissions"],
  t: (key: string) => string,
): PermissionGroup[] {
  if (!permissions) return [];
  const groups: PermissionGroup[] = [
    { id: "reads", label: t("canvases:permissionReads"), values: permissions.reads ?? [] },
    { id: "writes", label: t("canvases:permissionWrites"), values: permissions.writes ?? [] },
    { id: "events", label: t("canvases:permissionEvents"), values: permissions.events ?? [] },
    {
      id: "external-origins",
      label: t("canvases:permissionExternalOrigins"),
      values: permissions.external_origins ?? [],
    },
  ];
  if (permissions.shared_state) {
    groups.push({ id: "shared-state", label: t("canvases:sharedState"), values: [] });
  }
  return groups.filter((group) => group.values.length > 0 || group.id === "shared-state");
}

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

  const confirm = useCallback(async () => {
    if (
      !canvas ||
      !preview?.active_release_id ||
      !preview.permission_digest ||
      preview.grant_generation === undefined
    )
      return;
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
  }, [canvas, onCompleted, onOpenChange, preview, t]);

  const permissionGroups = buildPermissionGroups(preview?.permissions, t);

  return { preview, loading, confirming, error, confirm, permissionGroups };
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
  const { preview, loading, confirming, error, confirm, permissionGroups } = useCanvasPromotion(
    canvas,
    open,
    onOpenChange,
    onCompleted,
  );

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
                {permissionGroups.map((group) => (
                  <section key={group.id}>
                    <h3 className="font-medium">{group.label}</h3>
                    {group.values.length > 0 && (
                      <ul className="mt-1 space-y-1">
                        {group.values.map((permission) => (
                          <li key={permission} className="break-words text-muted-foreground">
                            {permission}
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                ))}
              </div>
            ) : (
              <p className="text-muted-foreground">{t("canvases:noAdditionalPermissions")}</p>
            )}
          </div>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            className="min-h-11 cursor-pointer md:min-h-7"
            onClick={() => onOpenChange(false)}
          >
            {t("common:cancel")}
          </Button>
          <Button
            className="min-h-11 cursor-pointer md:min-h-7"
            disabled={
              !preview ||
              !preview.active_release_id ||
              !preview.permission_digest ||
              preview.grant_generation === undefined ||
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
  const sourceTask =
    preview.source_task_id ?? preview.origin_task_id ?? canvas?.origin_task_id ?? canvas?.task_id;
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
      preview.source_actor_kind,
      "canvases:promotionSourceActor",
      "canvas-promotion-source-actor",
    ),
    row(sourceTask, "canvases:promotionSourceTask", "canvas-promotion-source-task"),
    row(
      preview.source_session_id ?? canvas?.created_by_session_id,
      "canvases:promotionSourceSession",
      "canvas-promotion-source-session",
    ),
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

type CanvasReleaseAction = "approve" | "reject" | "rollback";

function updateCanvasRelease(
  canvasId: string,
  releaseId: string,
  action: CanvasReleaseAction,
): Promise<Canvas> {
  if (action === "approve") return approveCanvasRelease(canvasId, releaseId);
  if (action === "reject") return rejectCanvasRelease(canvasId, releaseId);
  return rollbackCanvas(canvasId, releaseId);
}

function useCanvasReleases(
  canvas: Canvas | null,
  open: boolean,
  onChanged?: (canvas: Canvas) => void,
) {
  const { t } = useTranslation();
  const lifecycleRevision = useCanvasLifecycleRevision();
  const [releases, setReleases] = useState<CanvasRelease[]>([]);
  const [loading, setLoading] = useState(false);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open || !canvas) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    listCanvasReleases(canvas.id)
      .then((response) => {
        if (!cancelled) setReleases(response.releases ?? []);
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

  const releaseAction = useCallback(
    async (releaseId: string, action: CanvasReleaseAction) => {
      if (!canvas) return;
      setBusyId(releaseId);
      setError(null);
      try {
        const next = await updateCanvasRelease(canvas.id, releaseId, action);
        onChanged?.(next);
        const response = await listCanvasReleases(canvas.id);
        setReleases(response.releases ?? []);
      } catch (reason: unknown) {
        setError(canvasErrorMessage(reason, t, CANVAS_ACTION_FAILED_KEY));
      } finally {
        setBusyId(null);
      }
    },
    [canvas, onChanged, t],
  );

  return { releases, loading, busyId, error, releaseAction };
}

function CanvasReleaseValidationError({ code }: { code: string }) {
  const { t } = useTranslation();
  return (
    <p className="mt-1 text-destructive">
      {canvasErrorCodeMessage(code, t, CANVAS_ACTION_FAILED_KEY)}
    </p>
  );
}

function CanvasReleasePermissions({ release }: { release: CanvasRelease }) {
  const { t } = useTranslation();
  const permissionGroups = buildPermissionGroups(release.permissions, t);
  if (permissionGroups.length === 0) return null;

  return (
    <div
      className="mt-3 space-y-2 rounded-md bg-muted/40 p-2"
      data-testid={`canvas-release-permissions-${release.id}`}
    >
      <p className="font-medium">{t("canvases:permissionDeclaration")}</p>
      {permissionGroups.map((group) => (
        <section key={group.id}>
          <h3 className="font-medium">{group.label}</h3>
          {group.values.length > 0 && (
            <ul className="mt-1 space-y-1">
              {group.values.map((permission) => (
                <li key={permission} className="break-words text-muted-foreground">
                  {permission}
                </li>
              ))}
            </ul>
          )}
        </section>
      ))}
    </div>
  );
}

function CanvasReleaseProvenance({ release }: { release: CanvasRelease }) {
  const { t } = useTranslation();
  if (!release.source_actor_kind && !release.source_task_id && !release.source_session_id) {
    return null;
  }

  return (
    <dl className="mt-2 grid gap-1 text-xs">
      {release.source_actor_kind && (
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
          <dt className="text-muted-foreground">{t("canvases:promotionSourceActor")}</dt>
          <dd className="break-words">{release.source_actor_kind}</dd>
        </div>
      )}
      {release.source_task_id && (
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
          <dt className="text-muted-foreground">{t("canvases:promotionSourceTask")}</dt>
          <dd className="break-words">{release.source_task_id}</dd>
        </div>
      )}
      {release.source_session_id && (
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
          <dt className="text-muted-foreground">{t("canvases:promotionSourceSession")}</dt>
          <dd className="break-words">{release.source_session_id}</dd>
        </div>
      )}
    </dl>
  );
}

function CanvasReleaseActions({
  release,
  activeReleaseId,
  busyId,
  mutationsDisabled,
  releaseAction,
}: {
  release: CanvasRelease;
  activeReleaseId?: string;
  busyId: string | null;
  mutationsDisabled: boolean;
  releaseAction: (releaseId: string, action: CanvasReleaseAction) => void;
}) {
  const { t } = useTranslation();
  const busy = busyId === release.id;
  const pending = release.validation_status === "pending_permission";
  const canRollback = release.validation_status === "valid" && release.id !== activeReleaseId;

  if (!pending && !canRollback) return null;
  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {pending && (
        <>
          <Button
            className="min-h-11 cursor-pointer md:min-h-7"
            size="sm"
            disabled={busy || mutationsDisabled}
            onClick={() => releaseAction(release.id, "approve")}
          >
            {t("canvases:approveRelease")}
          </Button>
          <Button
            variant="outline"
            className="min-h-11 cursor-pointer md:min-h-7"
            size="sm"
            disabled={busy || mutationsDisabled}
            onClick={() => releaseAction(release.id, "reject")}
          >
            {t("canvases:rejectRelease")}
          </Button>
        </>
      )}
      {canRollback && (
        <Button
          variant="outline"
          className="min-h-11 cursor-pointer md:min-h-7"
          size="sm"
          disabled={busy || mutationsDisabled}
          onClick={() => releaseAction(release.id, "rollback")}
        >
          {t("canvases:rollbackRelease")}
        </Button>
      )}
    </div>
  );
}

function CanvasReleaseCard({
  release,
  activeReleaseId,
  busyId,
  mutationsDisabled,
  releaseAction,
}: {
  release: CanvasRelease;
  activeReleaseId?: string;
  busyId: string | null;
  mutationsDisabled: boolean;
  releaseAction: (releaseId: string, action: CanvasReleaseAction) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="rounded-md border p-3 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-medium">{release.id}</span>
        <span className="shrink-0 text-muted-foreground">
          {releaseStatusLabel(release.validation_status, t)}
        </span>
      </div>
      {release.validation_error && <CanvasReleaseValidationError code={release.validation_error} />}
      <CanvasReleasePermissions release={release} />
      {release.missing_permissions && release.missing_permissions.length > 0 && (
        <div className="mt-2 rounded-md border border-destructive/40 p-2">
          <p className="font-medium text-destructive">{t("canvases:missingPermissions")}</p>
          <ul className="mt-1 space-y-1 text-muted-foreground">
            {release.missing_permissions.map((permission) => (
              <li key={permission} className="break-words">
                {permission}
              </li>
            ))}
          </ul>
        </div>
      )}
      <CanvasReleaseProvenance release={release} />
      <CanvasReleaseActions
        release={release}
        activeReleaseId={activeReleaseId}
        busyId={busyId}
        mutationsDisabled={mutationsDisabled}
        releaseAction={releaseAction}
      />
    </div>
  );
}

export function CanvasReleaseDialog({
  canvas,
  open,
  onOpenChange,
  onChanged,
}: {
  canvas: Canvas | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChanged?: (canvas: Canvas) => void;
}) {
  const { t } = useTranslation();
  const { releases, loading, busyId, error, releaseAction } = useCanvasReleases(
    canvas,
    open,
    onChanged,
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="canvas-releases-dialog" className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("canvases:releasesAndPermissions")}</DialogTitle>
          <DialogDescription>{t("canvases:releasesDescription")}</DialogDescription>
        </DialogHeader>
        {loading && <p role="status">{t("canvases:loadingReleases")}</p>}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        {!loading && releases.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("canvases:noReleases")}</p>
        )}
        <div className="max-h-72 space-y-2 overflow-y-auto">
          {releases.map((release) => (
            <CanvasReleaseCard
              key={release.id}
              release={release}
              activeReleaseId={canvas?.active_release_id}
              busyId={busyId}
              mutationsDisabled={canvas?.status === "archived" || canvas?.status === "disabled"}
              releaseAction={(releaseId, action) => void releaseAction(releaseId, action)}
            />
          ))}
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            className="min-h-11 cursor-pointer md:min-h-7"
            onClick={() => onOpenChange(false)}
          >
            {t("common:close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
