"use client";

import { useEffect, useRef, useState } from "react";
import {
  IconClick,
  IconMessagePlus,
  IconScreenshot,
  IconTextScan2,
  IconX,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { PreviewCaptureMode } from "@/lib/preview-inspect-bridge";
import type { PreviewFeedbackDraft } from "@/hooks/use-preview-capture";
import {
  PreviewFeedbackCollection,
  previewFeedbackElementLabel,
  type PreviewFeedbackCollectionController,
} from "./preview-feedback-collection";

export type PreviewFeedbackController = PreviewFeedbackCollectionController & {
  mode: PreviewCaptureMode | null;
  draft: PreviewFeedbackDraft | null;
  draftComment: string;
  setDraftComment: (comment: string) => void;
  candidateLabel: string | null;
  captureError: "raster" | "upload" | "workspace" | null;
  isRasterizing: boolean;
  isUploading: boolean;
  isMutating: boolean;
  mutationError: string | null;
  startCapture: (mode: PreviewCaptureMode) => void;
  cancelCapture: () => void;
  discardDraft: () => void;
  saveDraft: (comment: string) => Promise<boolean>;
};

type PreviewFeedbackControlsProps = {
  capture: PreviewFeedbackController;
  enabled: boolean;
};

function draftEvidence(draft: PreviewFeedbackDraft) {
  if (draft.kind === "text") return draft.selected_text ?? "";
  if (draft.kind === "screenshot") return "";
  const element = draft.element_snapshot;
  if (!element) return "";
  return previewFeedbackElementLabel({ kind: "element", element_snapshot: element });
}

function CaptureChoices({
  capture,
  onChoose,
  touch,
}: {
  capture: PreviewFeedbackController;
  onChoose: (mode: PreviewCaptureMode) => void;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const buttonClass = touch ? "h-11 min-w-0 gap-1 px-2 text-xs" : "h-8 min-w-0 gap-1 px-2 text-xs";
  return (
    <div className="grid grid-cols-3 gap-1.5">
      <Button
        type="button"
        variant="outline"
        className={buttonClass}
        onClick={() => onChoose("text")}
        aria-label={t("task:previewSelectText")}
      >
        <IconTextScan2 className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="truncate">{t("task:previewCaptureText")}</span>
      </Button>
      <Button
        type="button"
        variant="outline"
        className={buttonClass}
        onClick={() => onChoose("element")}
        aria-label={t("task:previewSelectElement")}
      >
        <IconClick className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="truncate">{t("task:previewCaptureElement")}</span>
      </Button>
      <Button
        type="button"
        variant="outline"
        className={buttonClass}
        onClick={() => onChoose("screenshot")}
        aria-label={t("task:previewSelectScreenshot")}
      >
        <IconScreenshot className="h-4 w-4 shrink-0" aria-hidden="true" />
        <span className="truncate">{t("task:previewCaptureScreenshot")}</span>
      </Button>
      {capture.mode && (
        <Button
          type="button"
          variant="ghost"
          className={`${buttonClass} col-span-3`}
          onClick={capture.cancelCapture}
        >
          <IconX className="h-4 w-4" />
          {t("task:previewCancelSelection")}
        </Button>
      )}
    </div>
  );
}

function DraftEditor({ capture, touch }: { capture: PreviewFeedbackController; touch: boolean }) {
  const { t } = useTranslation();
  if (!capture.draft) return null;

  return (
    <section
      className="space-y-2 rounded-md border bg-muted/30 p-3"
      data-testid="preview-feedback-draft"
    >
      <div className="min-w-0">
        <p className="truncate text-xs text-muted-foreground">{capture.draft.page_route}</p>
        {capture.draft.kind === "screenshot" ? (
          <div className="mt-2 space-y-1">
            <img
              src={capture.draft.screenshot.previewUrl}
              alt={t("task:previewScreenshotAlt")}
              className="max-h-56 w-full rounded border bg-background object-contain"
            />
            <p className="text-xs text-muted-foreground">
              {t("task:previewScreenshotDimensions", {
                width: capture.draft.screenshot.width,
                height: capture.draft.screenshot.height,
              })}
            </p>
          </div>
        ) : (
          <p className="line-clamp-3 break-words font-mono text-xs">
            {draftEvidence(capture.draft)}
          </p>
        )}
      </div>
      <Textarea
        value={capture.draftComment}
        onChange={(event) => capture.setDraftComment(event.target.value)}
        aria-label={t("task:previewCommentSelection")}
        placeholder={t("task:previewCommentPlaceholder")}
        className="min-h-20 resize-y"
      />
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          className={touch ? "h-11" : "h-8"}
          onClick={capture.discardDraft}
          disabled={capture.isMutating}
        >
          {t("task:previewDiscardSelection")}
        </Button>
        <Button
          type="button"
          className={touch ? "h-11" : "h-8"}
          onClick={() => void capture.saveDraft(capture.draftComment)}
          disabled={!capture.draftComment.trim() || capture.isMutating || capture.isUploading}
        >
          {capture.isUploading
            ? t("task:previewUploadingScreenshot")
            : t("task:previewSaveFeedback")}
        </Button>
      </div>
    </section>
  );
}

function FeedbackSurface({
  capture,
  touch,
  onChoose,
}: {
  capture: PreviewFeedbackController;
  touch: boolean;
  onChoose: (mode: PreviewCaptureMode) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="min-h-0 space-y-4 overflow-y-auto p-3"
      data-vaul-no-drag={touch ? "" : undefined}
    >
      <CaptureChoices capture={capture} onChoose={onChoose} touch={touch} />
      {capture.isRasterizing && (
        <p role="status" className="text-xs text-muted-foreground">
          {t("task:previewRasterizing")}
        </p>
      )}
      <DraftEditor capture={capture} touch={touch} />
      {(capture.mutationError || capture.captureError) && (
        <p role="alert" className="text-xs text-destructive">
          {capture.captureError
            ? t("task:previewScreenshotFailed")
            : t("task:previewMutationFailed")}
        </p>
      )}
      <PreviewFeedbackCollection
        collection={capture}
        touch={touch}
        showEmpty={!capture.draft}
        showErrors={false}
      />
    </div>
  );
}

function Trigger({
  capture,
  enabled,
  touch,
  onClick,
}: PreviewFeedbackControlsProps & { touch: boolean; onClick?: () => void }) {
  const { t } = useTranslation();
  const label = t("task:previewAnnotateWithCount", { count: capture.items.length });
  return (
    <Button
      type="button"
      size="sm"
      variant={capture.mode ? "default" : "ghost"}
      disabled={!enabled}
      onClick={onClick}
      className={
        touch
          ? "h-11 min-w-11 cursor-pointer gap-1 px-2 text-xs"
          : "h-7 min-w-7 cursor-pointer gap-1 px-1.5 text-xs"
      }
      aria-label={label}
      title={label}
      data-testid="preview-feedback-trigger"
    >
      <IconMessagePlus className="h-4 w-4 shrink-0" aria-hidden="true" />
      {capture.items.length > 0 && (
        <span className="font-medium tabular-nums">{capture.items.length}</span>
      )}
    </Button>
  );
}

export function PreviewFeedbackControls({ capture, enabled }: PreviewFeedbackControlsProps) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const desktopRootRef = useRef<HTMLDivElement>(null);
  const [desktopMaxHeight, setDesktopMaxHeight] = useState(576);

  useEffect(() => {
    if (capture.draft || capture.isRasterizing || capture.captureError) setOpen(true);
  }, [capture.captureError, capture.draft, capture.isRasterizing]);

  useEffect(() => {
    if (!open || touch) return;
    const root = desktopRootRef.current;
    const updateHeight = () => {
      const bottom = root?.getBoundingClientRect().bottom ?? 0;
      setDesktopMaxHeight(Math.max(96, window.innerHeight - bottom - 8));
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (root && !event.composedPath().includes(root)) setOpen(false);
    };
    updateHeight();
    window.addEventListener("resize", updateHeight);
    document.addEventListener("keydown", closeOnEscape);
    document.addEventListener("pointerdown", closeOnOutsidePointer);
    return () => {
      window.removeEventListener("resize", updateHeight);
      document.removeEventListener("keydown", closeOnEscape);
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
    };
  }, [open, touch]);

  function choose(mode: PreviewCaptureMode) {
    capture.startCapture(mode);
    setOpen(false);
  }

  const candidateStatus = (
    <span className="sr-only" role="status" aria-live="polite">
      {capture.candidateLabel
        ? t("task:previewCandidate", { candidate: capture.candidateLabel })
        : ""}
    </span>
  );

  if (touch) {
    return (
      <Drawer open={open} onOpenChange={setOpen}>
        <Trigger capture={capture} enabled={enabled} touch onClick={() => setOpen(true)} />
        <DrawerContent
          className="max-h-[80dvh] overflow-hidden pb-[env(safe-area-inset-bottom)]"
          data-testid="preview-feedback-drawer"
        >
          <DrawerHeader className="border-b text-left">
            <DrawerTitle>{t("task:previewFeedbackTitle")}</DrawerTitle>
          </DrawerHeader>
          <FeedbackSurface capture={capture} touch onChoose={choose} />
        </DrawerContent>
        {candidateStatus}
      </Drawer>
    );
  }

  return (
    <div ref={desktopRootRef} className="relative">
      <Trigger
        capture={capture}
        enabled={enabled}
        touch={false}
        onClick={() => setOpen((current) => !current)}
      />
      {open && (
        <div
          role="dialog"
          aria-label={t("task:previewFeedbackTitle")}
          className="absolute right-0 top-[calc(100%+0.25rem)] z-50 flex w-96 max-w-[calc(100vw-1rem)] flex-col overflow-hidden rounded-lg bg-popover text-xs text-popover-foreground shadow-md ring-1 ring-foreground/10"
          style={{ maxHeight: Math.min(desktopMaxHeight, 576) }}
          data-testid="preview-feedback-popover"
        >
          <div className="border-b px-3 py-2">
            <h2 className="text-sm font-medium">{t("task:previewFeedbackTitle")}</h2>
          </div>
          <FeedbackSurface capture={capture} touch={false} onChoose={choose} />
        </div>
      )}
      {candidateStatus}
    </div>
  );
}
