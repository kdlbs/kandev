"use client";

import type { ReactNode } from "react";
import { IconEdit, IconListDetails, IconShare3, IconSparkles } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PanelHeaderOverflowMenu } from "@/components/task/panel-primitives";
import { type Canvas } from "@/lib/api/domains/canvas-api";

export function canvasLockHelp(canvas: Canvas, t: (key: string) => string): string {
  return canvas.status === "archived"
    ? t("canvases:archivedCanvasActionHelp")
    : t("canvases:disabledCanvasActionHelp");
}

export function canvasPromotionHelp(canvas: Canvas, t: (key: string) => string): string {
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

type CanvasDesktopOverflowActionsProps = {
  canvas: Canvas;
  editing: boolean;
  onEdit: () => void;
  onPromote: () => void;
  onReleases: () => void;
  onShare: () => void;
  omitPrimaryAction?: boolean;
};

export function CanvasDesktopPrimaryAction({
  canvas,
  editing,
  onEdit,
  onPromote,
}: Pick<CanvasDesktopOverflowActionsProps, "canvas" | "editing" | "onEdit" | "onPromote">) {
  const { t } = useTranslation();
  const lifecycleLocked = canvas.status === "archived" || canvas.status === "disabled";
  if (canvas.scope_kind === "workspace") {
    const editDisabled = editing || lifecycleLocked;
    return (
      <CanvasDesktopActionTooltip
        description={lifecycleLocked ? canvasLockHelp(canvas, t) : t("canvases:editCanvasHelp")}
        disabled={editDisabled}
        testId="canvas-action-edit-tooltip-trigger"
      >
        <Button
          variant="outline"
          size="sm"
          className="cursor-pointer"
          disabled={editDisabled}
          onClick={onEdit}
        >
          <IconEdit className="mr-1.5 h-3.5 w-3.5" />
          {t("canvases:editCanvas")}
        </Button>
      </CanvasDesktopActionTooltip>
    );
  }

  const promoteAvailable = canvas.active_release_status === "valid";
  const promoteDisabled = lifecycleLocked || !promoteAvailable;
  const promoteDescription = canvasPromotionHelp(canvas, t);
  return (
    <CanvasDesktopActionTooltip
      description={promoteDescription}
      disabled={promoteDisabled}
      testId="canvas-action-promote-tooltip-trigger"
    >
      <Button size="sm" className="cursor-pointer" disabled={promoteDisabled} onClick={onPromote}>
        <IconSparkles className="mr-1.5 h-3.5 w-3.5" />
        {t("canvases:promoteCanvas")}
      </Button>
    </CanvasDesktopActionTooltip>
  );
}

function CanvasDesktopOverflowMenuItemsContent({
  canvas,
  editing,
  onEdit,
  onPromote,
  onReleases,
  onShare,
  omitPrimaryAction,
  t,
}: CanvasDesktopOverflowActionsProps & { t: (key: string) => string }) {
  const lifecycleLocked = canvas.status === "archived" || canvas.status === "disabled";
  const promoteAvailable = canvas.scope_kind === "task" && canvas.active_release_status === "valid";
  const promoteDisabled = lifecycleLocked || !promoteAvailable;
  const promoteDescription = canvasPromotionHelp(canvas, t);
  const editDescription = lifecycleLocked
    ? canvasLockHelp(canvas, t)
    : t("canvases:editCanvasHelp");
  const editDisabled = editing || lifecycleLocked;
  const editHelpId = `canvas-edit-overflow-help-${canvas.id}`;
  const promoteHelpId = `canvas-promote-overflow-help-${canvas.id}`;

  return (
    <>
      {canvas.scope_kind === "workspace" && !omitPrimaryAction && (
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          disabled={editDisabled}
          title={editDescription}
          aria-describedby={editDisabled ? editHelpId : undefined}
          onSelect={onEdit}
        >
          <IconEdit className="size-4" />
          {t("canvases:editCanvas")}
          {editDisabled && (
            <span id={editHelpId} className="sr-only">
              {editDescription}
            </span>
          )}
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
      {canvas.scope_kind === "task" && !omitPrimaryAction && (
        <DropdownMenuItem
          className="cursor-pointer gap-2"
          disabled={promoteDisabled}
          title={promoteDescription}
          aria-describedby={promoteDisabled ? promoteHelpId : undefined}
          onSelect={onPromote}
        >
          <IconSparkles className="size-4" />
          {t("canvases:promoteCanvas")}
          {promoteDisabled && (
            <span id={promoteHelpId} className="sr-only">
              {promoteDescription}
            </span>
          )}
        </DropdownMenuItem>
      )}
    </>
  );
}

export function CanvasDesktopOverflowMenuItems(props: CanvasDesktopOverflowActionsProps) {
  const { t } = useTranslation();
  return <CanvasDesktopOverflowMenuItemsContent {...props} t={t} />;
}

export function CanvasDesktopOverflowActions(props: CanvasDesktopOverflowActionsProps) {
  const { t } = useTranslation();
  return (
    <PanelHeaderOverflowMenu label={t("canvases:canvasActions")}>
      <CanvasDesktopOverflowMenuItemsContent {...props} t={t} />
    </PanelHeaderOverflowMenu>
  );
}
