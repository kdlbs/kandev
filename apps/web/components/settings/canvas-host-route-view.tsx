"use client";

import { CanvasShareDialog } from "./canvas-share-dialog";
import { CanvasRenameDialog } from "./canvas-rename-dialog";
import { Button } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { IconPencil } from "@tabler/icons-react";
import { CanvasHostFrame } from "./canvas-host-frame";
import { useTranslation } from "react-i18next";
import {
  CanvasDesktopActions,
  CanvasDesktopOverflowActions,
  CanvasDesktopOverflowMenuItems,
  CanvasDesktopPrimaryAction,
  CanvasHostBody,
  CanvasHostDialogs,
  CanvasMobileActionsButton,
  MobileCanvasActions,
  type CanvasHostState,
} from "./canvas-host-components";
import { type Canvas } from "@/lib/api/domains/canvas-api";

type CanvasHostRouteViewProps = {
  canvasId: string;
  embedded: boolean;
  isMobile: boolean;
  canvas: Canvas | null;
  hostCanvases: Canvas[];
  runtimeUrl: string | null;
  state: CanvasHostState;
  error: string | null;
  menuOpen: boolean;
  promotionOpen: boolean;
  releasesOpen: boolean;
  shareOpen: boolean;
  renameOpen: boolean;
  editing: boolean;
  setMenuOpen: (open: boolean) => void;
  setPromotionOpen: (open: boolean) => void;
  setReleasesOpen: (open: boolean) => void;
  setShareOpen: (open: boolean) => void;
  setRenameOpen: (open: boolean) => void;
  onEdit: () => void;
  onPromote: () => void;
  onReleases: () => void;
  onShare: () => void;
  onRename: () => void;
  onSelectCanvas: (canvas: Canvas) => void;
  onRuntimeReady: () => void;
  onRuntimeError: () => void;
  onRetry: () => void;
  onPromotionCompleted: () => void;
  onChanged: () => void;
};

function CanvasHostRouteDialogs({
  canvas,
  promotionOpen,
  setPromotionOpen,
  releasesOpen,
  setReleasesOpen,
  shareOpen,
  setShareOpen,
  renameOpen,
  setRenameOpen,
  onPromotionCompleted,
  onChanged,
}: Pick<
  CanvasHostRouteViewProps,
  | "canvas"
  | "promotionOpen"
  | "setPromotionOpen"
  | "releasesOpen"
  | "setReleasesOpen"
  | "shareOpen"
  | "setShareOpen"
  | "renameOpen"
  | "setRenameOpen"
  | "onPromotionCompleted"
  | "onChanged"
>) {
  return (
    <>
      <CanvasHostDialogs
        canvas={canvas}
        promotionOpen={promotionOpen}
        onPromotionOpenChange={setPromotionOpen}
        releasesOpen={releasesOpen}
        onReleasesOpenChange={setReleasesOpen}
        onPromotionCompleted={onPromotionCompleted}
        onChanged={onChanged}
      />
      <CanvasShareDialog canvas={canvas} open={shareOpen} onOpenChange={setShareOpen} />
      <CanvasRenameDialog
        canvas={canvas}
        open={renameOpen}
        onOpenChange={setRenameOpen}
        onRenamed={onChanged}
      />
    </>
  );
}

function CanvasHostRouteBody({
  canvasId,
  title,
  state,
  runtimeUrl,
  error,
  onRuntimeReady,
  onRuntimeError,
  onRetry,
}: Pick<
  CanvasHostRouteViewProps,
  "canvasId" | "runtimeUrl" | "state" | "error" | "onRuntimeReady" | "onRuntimeError" | "onRetry"
> & { title: string }) {
  return (
    <CanvasHostBody
      canvasId={canvasId}
      title={title}
      state={state}
      runtimeUrl={runtimeUrl}
      error={error}
      onRuntimeReady={onRuntimeReady}
      onRuntimeError={onRuntimeError}
      onRetry={onRetry}
    />
  );
}

function CanvasHostRouteMobileActions({
  canvas,
  menuOpen,
  setMenuOpen,
  onEdit,
  onPromote,
  onReleases,
  onShare,
  onRename,
  editing,
  hostCanvases,
  onSelectCanvas,
}: Pick<
  CanvasHostRouteViewProps,
  | "canvas"
  | "menuOpen"
  | "setMenuOpen"
  | "onEdit"
  | "onPromote"
  | "onReleases"
  | "onShare"
  | "onRename"
  | "editing"
  | "onSelectCanvas"
> & { hostCanvases: Canvas[] }) {
  return (
    <MobileCanvasActions
      canvas={canvas}
      open={menuOpen}
      onOpenChange={setMenuOpen}
      onEdit={onEdit}
      onPromote={onPromote}
      onReleases={onReleases}
      onShare={onShare}
      onRename={onRename}
      editing={editing}
      canvases={hostCanvases}
      onSelectCanvas={onSelectCanvas}
    />
  );
}

function createCanvasDesktopActions(canvas: Canvas | null, props: CanvasHostRouteViewProps) {
  if (!canvas) {
    return {
      actions: null,
      overflowActions: null,
      overflowMenuItems: null,
      overflowPrimaryAction: null,
    };
  }

  const actionProps = {
    canvas,
    editing: props.editing,
    onEdit: props.onEdit,
    onPromote: props.onPromote,
    onReleases: props.onReleases,
    onShare: props.onShare,
  };
  return {
    actions: <CanvasDesktopActions {...actionProps} />,
    overflowActions: <CanvasDesktopOverflowActions {...actionProps} />,
    overflowMenuItems: <CanvasDesktopOverflowMenuItems {...actionProps} omitPrimaryAction />,
    overflowPrimaryAction: <CanvasDesktopPrimaryAction {...actionProps} />,
  };
}

export function CanvasHostRouteView(props: CanvasHostRouteViewProps) {
  const { canvas, isMobile, menuOpen, setMenuOpen } = props;
  const { t } = useTranslation();
  const title = canvas?.title || t("canvases:canvas");
  const desktopActions = createCanvasDesktopActions(canvas, props);
  const renameAction = canvas ? (
    <Button
      variant="ghost"
      size="icon"
      className={controlSizingClassName("icon")}
      aria-label={t("canvases:renameCanvas")}
      title={t("canvases:renameCanvas")}
      onClick={props.onRename}
      data-testid="canvas-rename-action"
    >
      <IconPencil className="size-4" />
    </Button>
  ) : null;
  const mobileActionsButton = isMobile ? (
    <CanvasMobileActionsButton menuOpen={menuOpen} onOpenActions={() => setMenuOpen(true)} />
  ) : null;
  const canvasBody = (
    <CanvasHostRouteBody
      canvasId={props.canvasId}
      title={title}
      state={props.state}
      runtimeUrl={props.runtimeUrl}
      error={props.error}
      onRuntimeReady={props.onRuntimeReady}
      onRuntimeError={props.onRuntimeError}
      onRetry={props.onRetry}
    />
  );
  const mobileActions = (
    <CanvasHostRouteMobileActions
      canvas={canvas}
      menuOpen={menuOpen}
      setMenuOpen={setMenuOpen}
      onEdit={props.onEdit}
      onPromote={props.onPromote}
      onReleases={props.onReleases}
      onShare={props.onShare}
      onRename={props.onRename}
      editing={props.editing}
      hostCanvases={props.hostCanvases}
      onSelectCanvas={props.onSelectCanvas}
    />
  );
  const dialogs = (
    <CanvasHostRouteDialogs
      canvas={canvas}
      promotionOpen={props.promotionOpen}
      setPromotionOpen={props.setPromotionOpen}
      releasesOpen={props.releasesOpen}
      setReleasesOpen={props.setReleasesOpen}
      shareOpen={props.shareOpen}
      setShareOpen={props.setShareOpen}
      renameOpen={props.renameOpen}
      setRenameOpen={props.setRenameOpen}
      onPromotionCompleted={props.onPromotionCompleted}
      onChanged={props.onChanged}
    />
  );

  return (
    <CanvasHostFrame
      embedded={props.embedded}
      isMobile={isMobile}
      title={title}
      menuOpen={menuOpen}
      setMenuOpen={setMenuOpen}
      desktopActions={desktopActions.actions}
      renameAction={renameAction}
      desktopOverflowActions={desktopActions.overflowActions}
      desktopOverflowMenuItems={desktopActions.overflowMenuItems}
      desktopOverflowPrimaryAction={desktopActions.overflowPrimaryAction}
      mobileActionsButton={mobileActionsButton}
      canvasBody={canvasBody}
      mobileActions={mobileActions}
      dialogs={dialogs}
    />
  );
}
