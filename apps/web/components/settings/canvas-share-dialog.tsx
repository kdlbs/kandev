"use client";

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { useCanvasShare } from "@/hooks/domains/canvas/use-canvas-share";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import { formatNumber } from "@/lib/i18n/formats";
import { CanvasShareHelp } from "./canvas-share-help";

export function CanvasShareDialog({
  canvas,
  open,
  onOpenChange,
}: {
  canvas: Canvas | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const share = useCanvasShare(canvas);

  useEffect(() => {
    if (!open) share.reset();
  }, [open]);

  const close = () => {
    void share.cancel();
    onOpenChange(false);
  };
  const prepare = () => void share.prepare();
  const body = (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-4 py-4">
      {!canvas?.active_release_id || canvas.active_release_status !== "valid" ? (
        <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
          {t("canvases:shareNoRelease")}
        </p>
      ) : (
        <>
          <p className="text-sm text-muted-foreground">
            {t("canvases:prepareDownloadsDescription")}
          </p>
          {!share.review && (
            <Button className="min-h-11 cursor-pointer" disabled={share.loading} onClick={prepare}>
              {share.loading ? t("canvases:sharing") : t("canvases:prepareDownloads")}
            </Button>
          )}
          {Boolean(share.error) && (
            <p role="alert" className="text-sm text-destructive">
              {t("canvases:shareFailed")}
            </p>
          )}
          {share.review && (
            <ExportReview
              review={share.review}
              loading={share.loading}
              onDownload={share.download}
            />
          )}
          <CanvasShareHelp review={share.review} />
        </>
      )}
    </div>
  );
  const footer = (
    <div className="flex shrink-0 flex-col-reverse gap-2 border-t px-4 py-3 md:flex-row md:justify-end">
      <Button variant="outline" className="min-h-11 cursor-pointer" onClick={close}>
        {t("common:cancel")}
      </Button>
    </div>
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="flex h-[100dvh] max-h-[100dvh] flex-col overflow-hidden">
          <DrawerHeader className="shrink-0 px-4 py-3 text-left">
            <DrawerTitle>{t("canvases:shareCanvas")}</DrawerTitle>
            <DrawerDescription>{t("canvases:shareCanvasDescription")}</DrawerDescription>
          </DrawerHeader>
          {body}
          <DrawerFooter className="shrink-0 p-0">{footer}</DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[92dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="shrink-0 px-4 pb-1 pt-3 text-left">
          <DialogTitle>{t("canvases:shareCanvas")}</DialogTitle>
          <DialogDescription>{t("canvases:shareCanvasDescription")}</DialogDescription>
        </DialogHeader>
        {body}
        <DialogFooter className="p-0">{footer}</DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ExportReview({
  review,
  loading,
  onDownload,
}: {
  review: NonNullable<ReturnType<typeof useCanvasShare>["review"]>;
  loading: boolean;
  onDownload: (kind: "bundle" | "source") => Promise<void>;
}) {
  const { t } = useTranslation();
  return (
    <section
      className="space-y-3 rounded-lg border border-border/70 p-4"
      data-testid="canvas-export-review"
    >
      <div>
        <h3 className="font-medium">{t("canvases:downloadsReady")}</h3>
        <p className="text-xs text-muted-foreground">
          {review.metadata.display_name ?? review.canvas_id} · v{review.metadata.version ?? ""}
        </p>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt className="text-muted-foreground">{t("canvases:packageId")}</dt>
        <dd className="break-all font-mono">{review.metadata.package_id ?? ""}</dd>
        <dt className="text-muted-foreground">{t("canvases:packageVersion")}</dt>
        <dd>{review.metadata.version ?? ""}</dd>
        <dt className="text-muted-foreground">{t("canvases:fileInventory")}</dt>
        <dd>{t("canvases:fileCount", { count: review.files.length })}</dd>
        <dt className="text-muted-foreground">{t("canvases:bundleSize")}</dt>
        <dd>{t("canvases:downloadSize", { size: formatNumber(review.bundle_bytes) })}</dd>
        <dt className="text-muted-foreground">{t("canvases:sourceSize")}</dt>
        <dd>{t("canvases:downloadSize", { size: formatNumber(review.source_bytes) })}</dd>
      </dl>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          className="min-h-11 flex-1 cursor-pointer"
          disabled={loading}
          onClick={() => void onDownload("bundle")}
        >
          {t("canvases:downloadBundle")}
        </Button>
        <Button
          variant="outline"
          className="min-h-11 flex-1 cursor-pointer"
          disabled={loading}
          onClick={() => void onDownload("source")}
        >
          {t("canvases:downloadSource")}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t("canvases:privateContentReminder")}</p>
    </section>
  );
}
