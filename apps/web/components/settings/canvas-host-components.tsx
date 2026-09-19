"use client";

import type { ReactNode } from "react";
import {
  IconEdit,
  IconExternalLink,
  IconLayoutGrid,
  IconListDetails,
  IconShare3,
  IconSparkles,
} from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { Button } from "@kandev/ui/button";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { PanelHeaderBarSplit, PanelHeaderOverflowMenu } from "@/components/task/panel-primitives";
import { CanvasPage } from "@/components/plugins/canvas-page";
import { canvasHref, type Canvas } from "@/lib/api/domains/canvas-api";
import { CanvasMobileActionsButton } from "./canvas-host-actions";
export { CanvasHostDialogs, CanvasMobileActionsButton } from "./canvas-host-actions";

export type CanvasHostState =
  | "loading_metadata"
  | "pending_first_release"
  | "pending_permission"
  | "loading_runtime"
  | "ready"
  | "offline"
  | "invalid_release"
  | "unavailable"
  | "archived";

const STATE_COPY: Record<CanvasHostState, { title: string; description: string }> = {
  loading_metadata: {
    title: "canvases:loadingCanvas",
    description: "canvases:loadingCanvasDescription",
  },
  pending_first_release: {
    title: "canvases:pendingFirstRelease",
    description: "canvases:pendingFirstReleaseDescription",
  },
  pending_permission: {
    title: "canvases:pendingPermission",
    description: "canvases:pendingPermissionDescription",
  },
  loading_runtime: {
    title: "canvases:loadingRuntime",
    description: "canvases:loadingRuntimeDescription",
  },
  ready: { title: "canvases:ready", description: "canvases:readyDescription" },
  offline: { title: "canvases:offline", description: "canvases:offlineDescription" },
  invalid_release: {
    title: "canvases:invalidRelease",
    description: "canvases:invalidReleaseDescription",
  },
  unavailable: { title: "canvases:unavailable", description: "canvases:unavailableDescription" },
  archived: { title: "canvases:archived", description: "canvases:archivedDescription" },
};

function canvasLockHelp(canvas: Canvas, t: (key: string) => string): string {
  return canvas.status === "archived"
    ? t("canvases:archivedCanvasActionHelp")
    : t("canvases:disabledCanvasActionHelp");
}

function canvasPromotionHelp(canvas: Canvas, t: (key: string) => string): string {
  if (canvas.status === "archived" || canvas.status === "disabled") {
    return canvasLockHelp(canvas, t);
  }
  if (canvas.scope_kind === "task" && canvas.active_release_status === "valid") {
    return t("canvases:promoteCanvasHelp");
  }
  return t("canvases:promoteCanvasUnavailable");
}

function CanvasDesktopActionTooltip({
  description,
  disabled,
  testId,
  children,
}: {
  description: string;
  disabled: boolean;
  testId: string;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={disabled ? "cursor-not-allowed" : undefined}
          data-testid={testId}
          tabIndex={disabled ? 0 : undefined}
        >
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent>{description}</TooltipContent>
    </Tooltip>
  );
}

function MobileCanvasAction({
  description,
  children,
  testId,
}: {
  description: string;
  children: ReactNode;
  testId: string;
}) {
  return (
    <div className="space-y-0.5 px-1">
      {children}
      <p className="px-3 text-xs text-muted-foreground" data-testid={testId}>
        {description}
      </p>
    </div>
  );
}

export function CanvasHostBody({
  canvasId,
  title,
  state,
  runtimeUrl,
  error,
  onRuntimeReady,
  onRuntimeError,
  onRetry,
}: {
  canvasId: string;
  title: string;
  state: CanvasHostState;
  runtimeUrl: string | null;
  error: string | null;
  onRuntimeReady: () => void;
  onRuntimeError: () => void;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden"
      data-testid="canvas-host-route"
    >
      {(state === "loading_runtime" || state === "ready") && runtimeUrl ? (
        <>
          {state === "ready" && (
            <span
              className="sr-only"
              role="status"
              aria-live="polite"
              data-testid="canvas-host-state"
            >
              <span data-testid="canvas-host-ready-announcement">{t(STATE_COPY.ready.title)}</span>
            </span>
          )}
          <CanvasPage
            key={`${canvasId}:${runtimeUrl}`}
            runtimeUrl={runtimeUrl}
            title={title}
            onLoad={onRuntimeReady}
            onError={onRuntimeError}
          />
        </>
      ) : (
        <CanvasHostStatePanel state={state} error={error} onRetry={onRetry} />
      )}
    </div>
  );
}

export function CanvasDesktopActions({
  canvas,
  editing,
  onEdit,
  onPromote,
  onReleases,
  onShare,
}: {
  canvas: Canvas;
  editing: boolean;
  onEdit: () => void;
  onPromote: () => void;
  onReleases: () => void;
  onShare: () => void;
}) {
  const { t } = useTranslation();
  const lifecycleLocked = canvas.status === "archived" || canvas.status === "disabled";
  const promoteAvailable = canvas.scope_kind === "task" && canvas.active_release_status === "valid";
  const promoteDisabled = lifecycleLocked || !promoteAvailable;
  const promoteDescription = canvasPromotionHelp(canvas, t);
  return (
    <div className="flex items-center gap-2">
      {canvas.scope_kind === "workspace" && (
        <CanvasDesktopActionTooltip
          description={lifecycleLocked ? canvasLockHelp(canvas, t) : t("canvases:editCanvasHelp")}
          disabled={editing || lifecycleLocked}
          testId="canvas-action-edit-tooltip-trigger"
        >
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            disabled={editing || lifecycleLocked}
            onClick={onEdit}
          >
            <IconEdit className="mr-1.5 h-3.5 w-3.5" />
            {t("canvases:editCanvas")}
          </Button>
        </CanvasDesktopActionTooltip>
      )}
      <CanvasDesktopActionTooltip
        description={t("canvases:releasesAndPermissionsHelp")}
        disabled={false}
        testId="canvas-action-releases-tooltip-trigger"
      >
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={onReleases}>
          <IconListDetails className="mr-1.5 h-3.5 w-3.5" />
          {t("canvases:releasesAndPermissions")}
        </Button>
      </CanvasDesktopActionTooltip>
      <CanvasDesktopActionTooltip
        description={t("canvases:shareCanvasDescription")}
        disabled={false}
        testId="canvas-action-share-tooltip-trigger"
      >
        <Button variant="outline" size="sm" className="cursor-pointer" onClick={onShare}>
          <IconShare3 className="mr-1.5 h-3.5 w-3.5" />
          {t("canvases:shareCanvas")}
        </Button>
      </CanvasDesktopActionTooltip>
      {canvas.scope_kind === "task" && (
        <CanvasDesktopActionTooltip
          description={promoteDescription}
          disabled={promoteDisabled}
          testId="canvas-action-promote-tooltip-trigger"
        >
          <Button
            size="sm"
            className="cursor-pointer"
            disabled={promoteDisabled}
            onClick={onPromote}
          >
            <IconSparkles className="mr-1.5 h-3.5 w-3.5" />
            {t("canvases:promoteCanvas")}
          </Button>
        </CanvasDesktopActionTooltip>
      )}
    </div>
  );
}

export function CanvasDesktopOverflowActions({
  canvas,
  editing,
  onEdit,
  onPromote,
  onReleases,
  onShare,
}: {
  canvas: Canvas;
  editing: boolean;
  onEdit: () => void;
  onPromote: () => void;
  onReleases: () => void;
  onShare: () => void;
}) {
  const { t } = useTranslation();
  const lifecycleLocked = canvas.status === "archived" || canvas.status === "disabled";
  const promoteAvailable = canvas.scope_kind === "task" && canvas.active_release_status === "valid";
  const promoteDisabled = lifecycleLocked || !promoteAvailable;
  const promoteDescription = canvasPromotionHelp(canvas, t);
  const editDescription = lifecycleLocked
    ? canvasLockHelp(canvas, t)
    : t("canvases:editCanvasHelp");

  return (
    <PanelHeaderOverflowMenu label={t("canvases:canvasActions")}>
      {canvas.scope_kind === "workspace" && (
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          disabled={editing || lifecycleLocked}
          title={editDescription}
          onSelect={onEdit}
        >
          <IconEdit className="size-4" />
          {t("canvases:editCanvas")}
        </DropdownMenuItem>
      )}
      <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onReleases}>
        <IconListDetails className="size-4" />
        {t("canvases:releasesAndPermissions")}
      </DropdownMenuItem>
      <DropdownMenuItem className="cursor-pointer gap-2" onSelect={onShare}>
        <IconShare3 className="size-4" />
        {t("canvases:shareCanvas")}
      </DropdownMenuItem>
      {canvas.scope_kind === "task" && (
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          disabled={promoteDisabled}
          title={promoteDescription}
          onSelect={onPromote}
        >
          <IconSparkles className="size-4" />
          {t("canvases:promoteCanvas")}
        </DropdownMenuItem>
      )}
    </PanelHeaderOverflowMenu>
  );
}

export function CanvasHostHeader({
  title,
  isMobile,
  menuOpen,
  onOpenActions,
  actions,
  overflowActions,
}: {
  title: string;
  isMobile: boolean;
  menuOpen: boolean;
  onOpenActions: () => void;
  actions?: ReactNode;
  overflowActions?: ReactNode;
}) {
  return (
    <PanelHeaderBarSplit
      data-testid="canvas-host-header"
      left={<span className="truncate text-sm font-medium">{title}</span>}
      right={
        <>
          {actions}
          {isMobile && (
            <CanvasMobileActionsButton menuOpen={menuOpen} onOpenActions={onOpenActions} />
          )}
        </>
      }
      overflow={!isMobile ? overflowActions : undefined}
      overflowAt={520}
    />
  );
}

export function CanvasHostStatePanel({
  state,
  error,
  onRetry,
}: {
  state: CanvasHostState;
  error: string | null;
  onRetry: () => void;
}) {
  const { t } = useTranslation();
  const copy = STATE_COPY[state];
  return (
    <div
      className="flex min-h-0 flex-1 items-center justify-center p-6 text-center"
      data-testid="canvas-host-state-panel"
    >
      <div className="max-w-md space-y-3">
        <h2 className="text-lg font-semibold" data-testid="canvas-host-state">
          {t(copy.title)}
        </h2>
        <p className="text-sm text-muted-foreground">{error || t(copy.description)}</p>
        {state !== "loading_metadata" && state !== "loading_runtime" && (
          <Button
            variant="outline"
            className={controlSizingClassName("standard", "cursor-pointer")}
            onClick={onRetry}
          >
            {t("canvases:retry")}
          </Button>
        )}
      </div>
    </div>
  );
}

function MobileCanvasPicker({
  canvases,
  canvas,
  onSelectCanvas,
  t,
}: {
  canvases: Canvas[];
  canvas: Canvas | null;
  onSelectCanvas: (canvas: Canvas) => void;
  t: (key: string) => string;
}) {
  return (
    <div className="mb-2 border-b pb-2" data-testid="canvas-mobile-picker">
      <p className="px-3 pb-1 text-xs font-medium text-muted-foreground">
        {t("canvases:canvases")}
      </p>
      {canvases.map((candidate) => (
        <Button
          key={candidate.id}
          variant="ghost"
          className="min-h-11 w-full justify-start cursor-pointer"
          disabled={candidate.id === canvas?.id}
          onClick={() => onSelectCanvas(candidate)}
          data-testid={`canvas-mobile-picker-item-${candidate.id}`}
        >
          <IconLayoutGrid className="mr-2 h-4 w-4" />
          <span className="truncate">{candidate.title}</span>
        </Button>
      ))}
    </div>
  );
}

function MobileCanvasEditAction({
  canvas,
  editing,
  onEdit,
  t,
}: {
  canvas: Canvas;
  editing: boolean;
  onEdit: () => void;
  t: (key: string) => string;
}) {
  const lifecycleLocked = canvas.status === "archived" || canvas.status === "disabled";
  return (
    <MobileCanvasAction
      description={lifecycleLocked ? canvasLockHelp(canvas, t) : t("canvases:editCanvasHelp")}
      testId="canvas-action-edit-help"
    >
      <Button
        variant="ghost"
        className="min-h-11 w-full justify-start cursor-pointer"
        disabled={editing || lifecycleLocked}
        onClick={onEdit}
      >
        <IconEdit className="mr-2 h-4 w-4" />
        {t("canvases:editCanvas")}
      </Button>
    </MobileCanvasAction>
  );
}

function MobileCanvasReleasesAction({
  onReleases,
  t,
}: {
  onReleases: () => void;
  t: (key: string) => string;
}) {
  return (
    <MobileCanvasAction
      description={t("canvases:releasesAndPermissionsHelp")}
      testId="canvas-action-releases-help"
    >
      <Button
        variant="ghost"
        className="min-h-11 w-full justify-start cursor-pointer"
        onClick={onReleases}
      >
        <IconListDetails className="mr-2 h-4 w-4" />
        {t("canvases:releasesAndPermissions")}
      </Button>
    </MobileCanvasAction>
  );
}

function MobileCanvasPromoteAction({
  canvas,
  lifecycleLocked,
  onPromote,
  t,
}: {
  canvas: Canvas;
  lifecycleLocked: boolean;
  onPromote: () => void;
  t: (key: string) => string;
}) {
  const promoteDisabled = lifecycleLocked || canvas.active_release_status !== "valid";
  return (
    <MobileCanvasAction
      description={canvasPromotionHelp(canvas, t)}
      testId="canvas-action-promote-help"
    >
      <Button
        variant="ghost"
        className="min-h-11 w-full justify-start cursor-pointer"
        disabled={promoteDisabled}
        onClick={onPromote}
      >
        <IconSparkles className="mr-2 h-4 w-4" />
        {t("canvases:promoteCanvas")}
      </Button>
    </MobileCanvasAction>
  );
}

function MobileCanvasShareAction({
  onShare,
  t,
}: {
  onShare: () => void;
  t: (key: string) => string;
}) {
  return (
    <MobileCanvasAction
      description={t("canvases:shareCanvasDescription")}
      testId="canvas-action-share-help"
    >
      <Button
        variant="ghost"
        className="min-h-11 w-full justify-start cursor-pointer"
        onClick={onShare}
      >
        <IconShare3 className="mr-2 h-4 w-4" />
        {t("canvases:shareCanvas")}
      </Button>
    </MobileCanvasAction>
  );
}

function MobileCanvasNewTabAction({ canvas, t }: { canvas: Canvas; t: (key: string) => string }) {
  return (
    <MobileCanvasAction
      description={t("canvases:openInNewTabHelp")}
      testId="canvas-action-new-tab-help"
    >
      <Button variant="ghost" className="min-h-11 w-full justify-start cursor-pointer" asChild>
        <a href={canvasHref(canvas.id)} target="_blank" rel="noreferrer">
          <IconExternalLink className="mr-2 h-4 w-4" />
          {t("canvases:openInNewTab")}
        </a>
      </Button>
    </MobileCanvasAction>
  );
}

export function MobileCanvasActions({
  canvas,
  canvases,
  open,
  onOpenChange,
  onEdit,
  onPromote,
  onReleases,
  onShare,
  onSelectCanvas,
  editing,
}: {
  canvas: Canvas | null;
  canvases: Canvas[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: () => void;
  onPromote: () => void;
  onReleases: () => void;
  onShare: () => void;
  onSelectCanvas: (canvas: Canvas) => void;
  editing: boolean;
}) {
  const { t } = useTranslation();
  const lifecycleLocked = canvas?.status === "archived" || canvas?.status === "disabled";
  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t("canvases:canvasActions")}
      description={canvas?.title}
      contentTestId="canvas-mobile-actions-sheet"
    >
      <div className="flex flex-col gap-1 pb-2">
        {canvases.length > 0 && (
          <MobileCanvasPicker
            canvases={canvases}
            canvas={canvas}
            onSelectCanvas={onSelectCanvas}
            t={t}
          />
        )}
        {canvas?.scope_kind === "workspace" && (
          <MobileCanvasEditAction canvas={canvas} editing={editing} onEdit={onEdit} t={t} />
        )}
        <MobileCanvasReleasesAction onReleases={onReleases} t={t} />
        {canvas && <MobileCanvasShareAction onShare={onShare} t={t} />}
        {canvas?.scope_kind === "task" && (
          <MobileCanvasPromoteAction
            canvas={canvas}
            lifecycleLocked={Boolean(lifecycleLocked)}
            onPromote={onPromote}
            t={t}
          />
        )}
        {canvas && <MobileCanvasNewTabAction canvas={canvas} t={t} />}
      </div>
    </MobilePickerSheet>
  );
}
