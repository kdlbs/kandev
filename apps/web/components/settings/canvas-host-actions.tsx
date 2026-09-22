"use client";

import { IconDots } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { CanvasPromotionDialog, CanvasReleaseDialog } from "./canvas-lifecycle-dialogs";

export function CanvasHostDialogs({
  canvas,
  promotionOpen,
  onPromotionOpenChange,
  releasesOpen,
  onReleasesOpenChange,
  onPromotionCompleted,
  onChanged,
}: {
  canvas: Canvas | null;
  promotionOpen: boolean;
  onPromotionOpenChange: (open: boolean) => void;
  releasesOpen: boolean;
  onReleasesOpenChange: (open: boolean) => void;
  onPromotionCompleted: () => void;
  onChanged: () => void;
}) {
  return (
    <>
      <CanvasPromotionDialog
        canvas={canvas?.scope_kind === "task" ? canvas : null}
        open={promotionOpen}
        onOpenChange={onPromotionOpenChange}
        onCompleted={onPromotionCompleted}
      />
      <CanvasReleaseDialog
        canvas={canvas}
        open={releasesOpen}
        onOpenChange={onReleasesOpenChange}
        onChanged={onChanged}
      />
    </>
  );
}

export function CanvasMobileActionsButton({
  menuOpen,
  onOpenActions,
}: {
  menuOpen: boolean;
  onOpenActions: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Button
      variant="outline"
      size="icon"
      className="h-11 w-11 shrink-0 cursor-pointer"
      aria-label={t("canvases:canvasActions")}
      aria-expanded={menuOpen}
      onClick={onOpenActions}
      data-testid="canvas-mobile-actions"
    >
      <IconDots className="h-4 w-4" />
    </Button>
  );
}
