"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  approveCanvasRelease,
  listCanvasReleases,
  rejectCanvasRelease,
  rollbackCanvas,
  type Canvas,
  type CanvasPermissionReview,
  type CanvasRelease,
} from "@/lib/api/domains/canvas-api";
import { canvasErrorCodeMessage, canvasErrorMessage } from "@/lib/api/domains/canvas-error-copy";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";
import {
  buildCanvasPermissionGroups,
  canvasReleaseStatusLabel,
  canvasSourceActorLabel,
  canvasSourceLabel,
  formatCanvasReleaseDate,
  type CanvasPermissionGroup,
} from "@/lib/canvas-permission-copy";

const CANVAS_ACTION_FAILED_KEY = "canvases:actionFailed";
const canvasActionClassName = controlSizingClassName("standard", "cursor-pointer");

export type CanvasReleaseAction = "approve" | "reject" | "rollback";

function hasUnsupportedPermissions(groups: CanvasPermissionGroup[]): boolean {
  return groups.some((group) => group.rows.some((row) => row.isUnsupported));
}

function CanvasPermissionSummary({
  permissions,
  missingPermissions,
}: {
  permissions: CanvasPermissionReview | undefined;
  missingPermissions?: string[];
}) {
  const { t } = useTranslation();
  const groups = buildCanvasPermissionGroups(permissions, missingPermissions, t);
  if (groups.length === 0) return null;

  return (
    <div
      className="space-y-3 rounded-md border bg-muted/20 p-3"
      data-testid="canvas-permission-summary"
    >
      <p className="font-medium">{t("canvases:permissionDeclaration")}</p>
      {groups.map((group) => (
        <section key={group.id}>
          <h3 className="font-medium">{group.label}</h3>
          <ul className="mt-1 space-y-2">
            {group.rows.map((row) => (
              <li key={row.id} className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1">
                <span className={row.isUnsupported ? "text-destructive" : undefined}>
                  {row.label}
                </span>
                {row.detail && (
                  <code className="break-all text-xs text-muted-foreground">{row.detail}</code>
                )}
                {row.isNew && (
                  <span className="rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary">
                    {t("canvases:newPermission")}
                  </span>
                )}
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

function CanvasReleaseValidationError({ code }: { code: string }) {
  const { t } = useTranslation();
  return (
    <p className="mt-1 text-destructive">
      {canvasErrorCodeMessage(code, t, CANVAS_ACTION_FAILED_KEY)}
    </p>
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
          <dd className="break-words">{canvasSourceActorLabel(release.source_actor_kind, t)}</dd>
        </div>
      )}
      {release.source_task_id && (
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
          <dt className="text-muted-foreground">{t("canvases:promotionSourceTask")}</dt>
          <dd className="break-words">{canvasSourceLabel(release.source_task_title, t)}</dd>
        </div>
      )}
      {release.source_session_id && (
        <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
          <dt className="text-muted-foreground">{t("canvases:promotionSourceSession")}</dt>
          <dd className="break-words">{canvasSourceLabel(release.source_session_name, t)}</dd>
        </div>
      )}
    </dl>
  );
}

function CanvasReleaseReview({
  release,
  activeReleaseId,
}: {
  release: CanvasRelease;
  activeReleaseId?: string;
}) {
  const { t } = useTranslation();
  const permissionGroups = buildCanvasPermissionGroups(
    release.permissions,
    release.missing_permissions,
    t,
  );
  return (
    <div className="space-y-4 text-sm" data-testid={`canvas-release-review-${release.id}`}>
      <dl className="grid gap-3 rounded-md bg-muted/40 p-3 sm:grid-cols-2">
        <div>
          <dt className="text-muted-foreground">{t("canvases:releaseDate")}</dt>
          <dd data-testid={`canvas-release-created-at-${release.id}`}>
            {formatCanvasReleaseDate(release.created_at, t)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">{t("canvases:releaseStatus")}</dt>
          <dd data-testid={`canvas-release-status-${release.id}`}>
            {canvasReleaseStatusLabel(release.validation_status, release.id, activeReleaseId, t)}
          </dd>
        </div>
      </dl>
      {release.validation_error && <CanvasReleaseValidationError code={release.validation_error} />}
      {permissionGroups.length > 0 ? (
        <div data-testid={`canvas-release-permissions-${release.id}`}>
          <CanvasPermissionSummary
            permissions={release.permissions}
            missingPermissions={release.missing_permissions}
          />
        </div>
      ) : (
        <p className="text-muted-foreground">{t("canvases:noAdditionalPermissions")}</p>
      )}
      <CanvasReleaseProvenance release={release} />
    </div>
  );
}

function useCanvasReleaseSelection(releases: CanvasRelease[], activeReleaseId?: string) {
  const [selectedReleaseId, setSelectedReleaseId] = useState<string | null>(null);
  const defaultReleaseId = useMemo(
    () =>
      releases.find((release) => release.validation_status === "pending_permission")?.id ??
      releases.find((release) => release.id === activeReleaseId)?.id ??
      releases[0]?.id ??
      null,
    [activeReleaseId, releases],
  );

  useEffect(() => {
    setSelectedReleaseId((current) =>
      current && releases.some((release) => release.id === current) ? current : defaultReleaseId,
    );
  }, [defaultReleaseId, releases]);

  const effectiveSelectedReleaseId = selectedReleaseId ?? defaultReleaseId;
  const selectedRelease = useMemo(
    () => releases.find((release) => release.id === effectiveSelectedReleaseId) ?? null,
    [effectiveSelectedReleaseId, releases],
  );
  return {
    selectedRelease,
    selectedReleaseId: effectiveSelectedReleaseId,
    setSelectedReleaseId,
  };
}

function CanvasReleaseSelector({
  releases,
  selectedReleaseId,
  activeReleaseId,
  onChange,
}: {
  releases: CanvasRelease[];
  selectedReleaseId: string | null;
  activeReleaseId?: string;
  onChange: (releaseId: string) => void;
}) {
  const { t } = useTranslation();
  if (releases.length === 0) return null;
  return (
    <label className="mt-3 grid gap-1">
      <span className="text-muted-foreground">{t("canvases:selectRelease")}</span>
      <select
        aria-label={t("canvases:selectRelease")}
        className="min-h-10 rounded-md border border-input bg-background px-3 py-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring sm:min-h-8"
        value={selectedReleaseId ?? ""}
        onChange={(event) => onChange(event.target.value)}
      >
        {releases.map((release) => (
          <option key={release.id} value={release.id}>
            {t("canvases:releaseOption", {
              date: formatCanvasReleaseDate(release.created_at, t),
              status: canvasReleaseStatusLabel(
                release.validation_status,
                release.id,
                activeReleaseId,
                t,
              ),
            })}
          </option>
        ))}
      </select>
    </label>
  );
}

function CanvasReleaseReviewBody({
  loading,
  error,
  releases,
  selectedRelease,
  activeReleaseId,
}: {
  loading: boolean;
  error: string | null;
  releases: CanvasRelease[];
  selectedRelease: CanvasRelease | null;
  activeReleaseId?: string;
}) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="canvas-release-review-scroll"
      className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4 sm:p-6"
    >
      {loading && <p role="status">{t("canvases:loadingReleases")}</p>}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {!loading && releases.length === 0 && (
        <p className="text-sm text-muted-foreground">{t("canvases:noReleases")}</p>
      )}
      {!loading && selectedRelease && (
        <CanvasReleaseReview release={selectedRelease} activeReleaseId={activeReleaseId} />
      )}
    </div>
  );
}

function CanvasReleaseFooter({
  isMobile,
  selectedRelease,
  pending,
  canRollback,
  selectedBusy,
  mutationsDisabled,
  unsupportedPermissions,
  onClose,
  onAction,
}: {
  isMobile: boolean;
  selectedRelease: CanvasRelease | null;
  pending: boolean;
  canRollback: boolean;
  selectedBusy: boolean;
  mutationsDisabled: boolean;
  unsupportedPermissions: boolean;
  onClose: () => void;
  onAction: (action: CanvasReleaseAction) => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter
      className={
        isMobile
          ? "shrink-0 grid grid-cols-2 border-t border-border/70 p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
          : "shrink-0 flex-row border-t border-border/70 p-4 sm:p-6"
      }
    >
      <Button variant="outline" className={canvasActionClassName} onClick={onClose}>
        {t("common:close")}
      </Button>
      {pending && selectedRelease && (
        <>
          <Button
            variant="outline"
            className={canvasActionClassName}
            disabled={selectedBusy || mutationsDisabled}
            onClick={() => onAction("reject")}
          >
            {t("canvases:rejectRelease")}
          </Button>
          <Button
            className={canvasActionClassName}
            disabled={selectedBusy || mutationsDisabled || unsupportedPermissions}
            onClick={() => onAction("approve")}
          >
            {t("canvases:approveRelease")}
          </Button>
        </>
      )}
      {canRollback && selectedRelease && (
        <Button
          variant="outline"
          className={canvasActionClassName}
          disabled={selectedBusy || mutationsDisabled}
          onClick={() => onAction("rollback")}
        >
          {t("canvases:rollbackRelease")}
        </Button>
      )}
    </DialogFooter>
  );
}

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
    setReleases([]);
    setBusyId(null);
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
  const { isMobile } = useResponsiveBreakpoint();
  const { releases, loading, busyId, error, releaseAction } = useCanvasReleases(
    canvas,
    open,
    onChanged,
  );
  const { selectedRelease, selectedReleaseId, setSelectedReleaseId } = useCanvasReleaseSelection(
    releases,
    canvas?.active_release_id,
  );
  const selectedPermissionGroups = buildCanvasPermissionGroups(
    selectedRelease?.permissions,
    selectedRelease?.missing_permissions,
    t,
  );
  const pending = selectedRelease?.validation_status === "pending_permission";
  const canRollback =
    selectedRelease?.validation_status === "valid" &&
    selectedRelease.id !== canvas?.active_release_id;
  const surfaceClassName = isMobile
    ? "!left-0 !top-0 !h-dvh !max-h-dvh !w-screen !max-w-none !translate-x-0 !translate-y-0 flex flex-col gap-0 overflow-hidden rounded-none p-0 [padding-top:max(1rem,env(safe-area-inset-top))]"
    : "flex h-[min(90dvh,48rem)] max-h-[calc(100dvh-2rem)] w-full max-w-[48rem] flex-col gap-0 overflow-hidden p-0";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="canvas-releases-dialog"
        className={surfaceClassName}
        showCloseButton={false}
      >
        <DialogHeader className="shrink-0 border-b border-border/70 p-4 text-left sm:p-6">
          <DialogTitle>{t("canvases:releasesAndPermissions")}</DialogTitle>
          <DialogDescription>{t("canvases:releasesDescription")}</DialogDescription>
          <CanvasReleaseSelector
            releases={releases}
            selectedReleaseId={selectedReleaseId}
            activeReleaseId={canvas?.active_release_id}
            onChange={setSelectedReleaseId}
          />
        </DialogHeader>
        <CanvasReleaseReviewBody
          loading={loading}
          error={error}
          releases={releases}
          selectedRelease={selectedRelease}
          activeReleaseId={canvas?.active_release_id}
        />
        <CanvasReleaseFooter
          isMobile={isMobile}
          selectedRelease={selectedRelease}
          pending={pending}
          canRollback={canRollback}
          selectedBusy={selectedRelease ? busyId === selectedRelease.id : false}
          mutationsDisabled={canvas?.status === "archived" || canvas?.status === "disabled"}
          unsupportedPermissions={hasUnsupportedPermissions(selectedPermissionGroups)}
          onClose={() => onOpenChange(false)}
          onAction={(action) => {
            if (selectedRelease) void releaseAction(selectedRelease.id, action);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}
