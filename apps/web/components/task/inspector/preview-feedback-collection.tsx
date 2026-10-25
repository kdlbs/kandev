"use client";

import { useEffect, useRef, useState, type MutableRefObject, type ReactNode } from "react";
import { IconEdit, IconMessageDots, IconTrash, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DrawerClose } from "@kandev/ui/drawer";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { Textarea } from "@kandev/ui/textarea";
import { useTranslation } from "react-i18next";
import { createFocusReturnHandler } from "@/lib/dialog-focus-return";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import type { TaskPreviewFeedback, TaskPreviewFeedbackSnapshot } from "@/lib/types/http";

export type PreviewFeedbackCollectionController = {
  items: TaskPreviewFeedback[];
  snapshot?: TaskPreviewFeedbackSnapshot;
  isLoading?: boolean;
  loadError?: string | null;
  isMutating: boolean;
  mutationError: string | null;
  update: (
    id: string,
    comment: string,
    expectedVersion: number,
  ) => Promise<TaskPreviewFeedbackSnapshot | null>;
  remove: (id: string, expectedVersion: number) => Promise<TaskPreviewFeedbackSnapshot | null>;
  clear: (expectedRevision: number) => Promise<TaskPreviewFeedbackSnapshot | null>;
};

export function previewFeedbackElementLabel(
  item: Pick<TaskPreviewFeedback, "kind" | "element_snapshot">,
) {
  const element = item.element_snapshot;
  if (!element) return "";
  let label = element.tag;
  if (element.id) label += `#${element.id}`;
  else if (element.classes[0]) label += `.${element.classes[0]}`;
  return `<${label}>`;
}

function feedbackEvidence(item: TaskPreviewFeedback) {
  if (item.kind === "text") return item.selected_text ?? "";
  if (item.kind === "element") return previewFeedbackElementLabel(item);
  return item.screenshot_attachment?.name ?? "";
}

function FeedbackEvidence({ item }: { item: TaskPreviewFeedback }) {
  const { t } = useTranslation();
  if (item.kind === "screenshot" && item.screenshot_attachment) {
    return (
      <div className="mt-1 space-y-1">
        <img
          src={attachmentContentUrl(item.screenshot_attachment.attachment_id)}
          alt={t("task:previewScreenshotAlt")}
          className="max-h-40 w-full rounded border bg-background object-contain"
        />
        <p className="truncate font-mono text-xs">{item.screenshot_attachment.name}</p>
      </div>
    );
  }
  return <p className="line-clamp-2 break-words font-mono text-xs">{feedbackEvidence(item)}</p>;
}

function PendingFeedbackItem({
  item,
  collection,
  touch,
}: {
  item: TaskPreviewFeedback;
  collection: PreviewFeedbackCollectionController;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [editComment, setEditComment] = useState("");
  const actionClass = touch ? "h-11 min-w-11" : "h-7 min-w-7";

  function startEditing() {
    if (collection.isMutating) return;
    setEditing(true);
    setEditComment(item.comment);
  }

  async function saveEditing() {
    const comment = editComment.trim();
    if (!comment || collection.isMutating) return;
    const result = await collection.update(item.id, comment, item.version);
    if (result) setEditing(false);
  }

  return (
    <li className="rounded-md border p-3" data-testid="preview-feedback-item">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs text-muted-foreground">
            {item.source_label} · {item.page_route}
          </p>
          <FeedbackEvidence item={item} />
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={actionClass}
          onClick={startEditing}
          disabled={collection.isMutating}
          aria-label={t("task:previewEditFeedback")}
        >
          <IconEdit className="h-4 w-4" />
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={actionClass}
          onClick={() => void collection.remove(item.id, item.version)}
          disabled={collection.isMutating}
          aria-label={t("task:previewDeleteFeedback")}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
      {editing ? (
        <div className="mt-2 space-y-2">
          <Textarea
            value={editComment}
            onChange={(event) => setEditComment(event.target.value)}
            aria-label={t("task:previewEditComment")}
            className="min-h-20"
          />
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              className={actionClass}
              onClick={() => setEditing(false)}
              disabled={collection.isMutating}
            >
              {t("task:previewCancel")}
            </Button>
            <Button
              type="button"
              className={actionClass}
              onClick={() => void saveEditing()}
              disabled={!editComment.trim() || collection.isMutating}
            >
              {t("task:previewSaveChanges")}
            </Button>
          </div>
        </div>
      ) : (
        <p className="mt-2 break-words text-xs">{item.comment}</p>
      )}
    </li>
  );
}

function CollectionHeader({
  collection,
  touch,
}: {
  collection: PreviewFeedbackCollectionController;
  touch: boolean;
}) {
  const { t } = useTranslation();
  const [confirmClear, setConfirmClear] = useState(false);
  const actionClass = touch ? "h-11 min-w-11" : "h-7 min-w-7";

  async function confirmClearCollection() {
    if (collection.isMutating) return;
    const result = await collection.clear(collection.snapshot?.revision ?? 0);
    if (result) setConfirmClear(false);
  }

  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t("task:previewPendingFeedback")}</h3>
        {collection.items.length > 0 && !confirmClear && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className={actionClass}
            onClick={() => setConfirmClear(true)}
            disabled={collection.isMutating}
            aria-label={t("task:previewClearAll")}
          >
            <IconTrash className="h-4 w-4" />
          </Button>
        )}
      </div>
      {confirmClear && (
        <div className="rounded-md border border-destructive/30 p-3">
          <p className="text-xs text-muted-foreground">
            {t("task:previewConfirmClearDescription")}
          </p>
          <div className="mt-2 flex justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              className={actionClass}
              onClick={() => setConfirmClear(false)}
              disabled={collection.isMutating}
            >
              {t("task:previewCancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              className={actionClass}
              onClick={() => void confirmClearCollection()}
              disabled={collection.isMutating}
            >
              {t("task:previewClearAll")}
            </Button>
          </div>
        </div>
      )}
    </>
  );
}

function CollectionStatus({
  collection,
  showEmpty,
  showErrors,
}: {
  collection: PreviewFeedbackCollectionController;
  showEmpty: boolean;
  showErrors: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      {collection.isLoading && collection.items.length === 0 && (
        <p role="status" className="py-4 text-center text-xs text-muted-foreground">
          {t("task:previewLoadingFeedback")}
        </p>
      )}
      {showErrors && collection.loadError && (
        <p role="alert" className="text-xs text-destructive">
          {t("task:previewLoadFailed")}
        </p>
      )}
      {showErrors && collection.mutationError && (
        <p role="alert" className="text-xs text-destructive">
          {t("task:previewMutationFailed")}
        </p>
      )}
      {showEmpty && !collection.isLoading && collection.items.length === 0 && (
        <p className="py-4 text-center text-xs text-muted-foreground">
          {t("task:previewPendingEmpty")}
        </p>
      )}
    </>
  );
}

export function PreviewFeedbackCollection({
  collection,
  touch,
  showEmpty = true,
  showErrors = true,
}: {
  collection: PreviewFeedbackCollectionController;
  touch: boolean;
  showEmpty?: boolean;
  showErrors?: boolean;
}) {
  return (
    <section className="space-y-2" data-testid="preview-feedback-collection">
      <CollectionHeader collection={collection} touch={touch} />
      <CollectionStatus collection={collection} showEmpty={showEmpty} showErrors={showErrors} />
      <ul className="space-y-2">
        {collection.items.map((item) => (
          <PendingFeedbackItem key={item.id} item={item} collection={collection} touch={touch} />
        ))}
      </ul>
    </section>
  );
}

type CollectionSurfaceProps = {
  taskId: string | null | undefined;
  collection: PreviewFeedbackCollectionController;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  showTrigger?: boolean;
};

function CollectionTrigger({
  collection,
  touch,
  title,
  countLabel,
  showTrigger,
  focusReturnRef,
  onOpen,
}: {
  collection: PreviewFeedbackCollectionController;
  touch: boolean;
  title: string;
  countLabel: string;
  showTrigger: boolean;
  focusReturnRef: MutableRefObject<HTMLElement | null>;
  onOpen: () => void;
}) {
  if (!showTrigger) return null;
  return (
    <Button
      ref={(element) => {
        if (element && !focusReturnRef.current) focusReturnRef.current = element;
      }}
      type="button"
      variant="outline"
      onClick={onOpen}
      className={touch ? "h-11 min-w-11 gap-1 px-2 text-xs" : "h-8 gap-1 px-2 text-xs"}
      aria-label={countLabel}
      title={title}
      data-testid="preview-feedback-collection-trigger"
    >
      <IconMessageDots className="h-4 w-4 shrink-0" aria-hidden="true" />
      <span>{title}</span>
      <span className="font-medium tabular-nums">{collection.items.length}</span>
    </Button>
  );
}

function TouchCollectionSurface({
  collection,
  open,
  onOpenChange,
  title,
  closeFocus,
}: {
  collection: PreviewFeedbackCollectionController;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  closeFocus: (event: Event) => void;
}) {
  const { t } = useTranslation();
  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      contentTestId="preview-feedback-collection-body"
      confirmationHost
      onCloseAutoFocus={closeFocus}
      headerAction={
        <DrawerClose asChild>
          <Button
            type="button"
            variant="ghost"
            className="h-11 min-w-11"
            aria-label={t("common:close")}
          >
            <IconX className="h-4 w-4" />
          </Button>
        </DrawerClose>
      }
    >
      <PreviewFeedbackCollection collection={collection} touch />
    </MobilePickerSheet>
  );
}

function DesktopCollectionSurface({
  collection,
  open,
  onOpenChange,
  title,
  closeFocus,
  trigger,
}: {
  collection: PreviewFeedbackCollectionController;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  closeFocus: (event: Event) => void;
  trigger: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <div className="relative">
        {trigger}
        <PopoverAnchor asChild>
          <span aria-hidden="true" className="absolute bottom-0 right-0 h-px w-px" />
        </PopoverAnchor>
        <PopoverContent
          side="top"
          align="end"
          portal={false}
          onCloseAutoFocus={closeFocus}
          className="w-96 max-w-[calc(100vw-1rem)] overflow-hidden p-0"
          data-testid="preview-feedback-collection-popover"
        >
          <div className="flex items-center justify-between gap-2 border-b px-3 py-2">
            <h2 className="text-sm font-medium">{title}</h2>
            <Button
              type="button"
              variant="ghost"
              className="h-7 min-w-7"
              onClick={() => onOpenChange(false)}
              aria-label={t("common:close")}
            >
              <IconX className="h-4 w-4" />
            </Button>
          </div>
          <div className="max-h-[min(36rem,calc(100dvh-1rem))] overflow-y-auto p-3">
            <PreviewFeedbackCollection collection={collection} touch={false} />
          </div>
        </PopoverContent>
      </div>
    </Popover>
  );
}

export function PreviewFeedbackCollectionSurface({
  taskId,
  collection,
  open,
  onOpenChange,
  showTrigger = false,
}: CollectionSurfaceProps) {
  const { t } = useTranslation();
  const touch = useTouchDrawer();
  const focusReturnRef = useRef<HTMLElement | null>(null);
  const title = t("task:previewFeedbackTitle");
  const countLabel = t("task:previewFeedbackCount", { count: collection.items.length });

  useEffect(() => {
    if (!open) return;
    if (document.activeElement instanceof HTMLElement) {
      focusReturnRef.current = document.activeElement;
    }
  }, [open]);

  if (!taskId) return null;

  const rememberFocusAndOpen = () => {
    if (document.activeElement instanceof HTMLElement) {
      focusReturnRef.current = document.activeElement;
    }
    onOpenChange(true);
  };
  const trigger = (
    <CollectionTrigger
      collection={collection}
      touch={touch}
      title={title}
      countLabel={countLabel}
      showTrigger={showTrigger}
      focusReturnRef={focusReturnRef}
      onOpen={rememberFocusAndOpen}
    />
  );
  const closeFocus = createFocusReturnHandler(focusReturnRef);

  if (touch) {
    return (
      <>
        {trigger}
        <TouchCollectionSurface
          collection={collection}
          open={open}
          onOpenChange={onOpenChange}
          title={title}
          closeFocus={closeFocus}
        />
      </>
    );
  }

  return (
    <>
      <DesktopCollectionSurface
        collection={collection}
        open={open}
        onOpenChange={onOpenChange}
        title={title}
        closeFocus={closeFocus}
        trigger={trigger}
      />
    </>
  );
}
