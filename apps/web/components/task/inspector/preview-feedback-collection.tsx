"use client";

import { useEffect, useRef, useState, type MutableRefObject, type ReactNode } from "react";
import { IconEdit, IconMessageDots, IconTrash, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DrawerClose } from "@kandev/ui/drawer";
import { Popover, PopoverAnchor, PopoverContent } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { createFocusReturnHandler } from "@/lib/dialog-focus-return";
import { cn } from "@/lib/utils";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import {
  SelectionCommentSurface,
  isInsideSelectionCommentSurface,
} from "@/components/shared/selection-comment/selection-comment-surface";
import {
  PreviewFeedbackEvidence,
  evidenceForItem,
  evidenceText,
} from "./preview-feedback-evidence";
import { PreviewUnsavedDrafts, type PreviewUnsavedDraftsProps } from "./preview-unsaved-drafts";
import { CaptureNumber, type PreviewCommentList } from "./preview-comment-list";

export type { PreviewCommentList } from "./preview-comment-list";
import type { SelectionCommentComposerProps } from "@/components/shared/selection-comment/selection-comment-composer";
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

/** Keeps the host Popover open while the user works in a floating comment surface. */
function keepOpenForCommentSurface(event: {
  target: EventTarget | null;
  preventDefault: () => void;
}) {
  if (isInsideSelectionCommentSurface(event.target)) event.preventDefault();
}

/** Comment form props for editing a saved item in the floating comment box. */
function usePendingEditorProps(
  item: TaskPreviewFeedback,
  collection: PreviewFeedbackCollectionController,
  onClose: () => void,
): SelectionCommentComposerProps {
  const { t } = useTranslation();
  const evidence = evidenceForItem(item);
  return {
    selectedText: evidenceText(evidence) || item.page_route,
    contextLabel: item.page_route,
    evidence: <PreviewFeedbackEvidence evidence={evidence} />,
    editingComment: item.comment,
    inputLabel: t("task:previewEditComment"),
    errorMessage: collection.mutationError ? t("task:previewMutationFailed") : null,
    testId: "preview-feedback-edit",
    inputTestId: "preview-feedback-edit-input",
    addButtonTestId: "preview-feedback-edit-save",
    onAdd: async (comment) => Boolean(await collection.update(item.id, comment, item.version)),
    onDelete: async () => Boolean(await collection.remove(item.id, item.version)),
    onClose,
    onCancel: onClose,
  };
}

export function PendingFeedbackEditor({
  item,
  collection,
  position,
  onClose,
}: {
  item: TaskPreviewFeedback;
  collection: PreviewFeedbackCollectionController;
  position: { x: number; y: number };
  onClose: () => void;
}) {
  const props = usePendingEditorProps(item, collection, onClose);
  return <SelectionCommentSurface {...props} position={position} animated={false} />;
}

function PendingFeedbackItem({
  item,
  collection,
  touch,
  list,
}: {
  item: TaskPreviewFeedback;
  collection: PreviewFeedbackCollectionController;
  touch: boolean;
  list?: PreviewCommentList;
}) {
  const { t } = useTranslation();
  const [editAnchor, setEditAnchor] = useState<{ x: number; y: number } | null>(null);
  const actionClass = touch ? "h-11 min-w-11" : "h-7 min-w-7";

  return (
    <li
      className={cn(
        "rounded-md border p-3",
        list && "cursor-pointer",
        (list?.selectedId === item.id || list?.activeId === item.id) && "border-primary",
      )}
      data-testid="preview-feedback-item"
      data-selected={list?.selectedId === item.id ? "true" : undefined}
      data-active={list?.activeId === item.id ? "true" : undefined}
      onClick={list ? () => list.onSelect(item.id) : undefined}
    >
      <div className="flex items-start gap-2">
        {list && <CaptureNumber value={list.numbers.get(item.id)} />}
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs text-muted-foreground">
            {item.source_label} · {item.page_route}
          </p>
          <div className="mt-1">
            <PreviewFeedbackEvidence evidence={evidenceForItem(item)} />
          </div>
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className={actionClass}
          onClick={(event) => {
            event.stopPropagation();
            if (list) return list.onOpen(item.id);
            const bounds = event.currentTarget.getBoundingClientRect();
            setEditAnchor({ x: bounds.left, y: bounds.bottom });
          }}
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
      <p className="mt-2 break-words text-xs">{item.comment}</p>
      {editAnchor && (
        <PendingFeedbackEditor
          item={item}
          collection={collection}
          position={editAnchor}
          onClose={() => setEditAnchor(null)}
        />
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
  drafts,
  list,
}: {
  collection: PreviewFeedbackCollectionController;
  touch: boolean;
  showEmpty?: boolean;
  showErrors?: boolean;
  drafts?: PreviewUnsavedDraftsProps;
  list?: PreviewCommentList;
}) {
  return (
    <section className="space-y-2" data-testid="preview-feedback-collection">
      {drafts && <PreviewUnsavedDrafts {...drafts} touch={touch} list={list} />}
      <CollectionHeader collection={collection} touch={touch} />
      <CollectionStatus collection={collection} showEmpty={showEmpty} showErrors={showErrors} />
      <ul className="space-y-2">
        {collection.items.map((item) => (
          <PendingFeedbackItem
            key={item.id}
            item={item}
            collection={collection}
            touch={touch}
            list={list}
          />
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
  /** Replaces the default trigger button; the host opens the surface itself. */
  trigger?: ReactNode;
  side?: "top" | "bottom";
  /** Renders the desktop popover in a portal, outside clipping or pointer-transparent hosts. */
  portal?: boolean;
  drafts?: PreviewUnsavedDraftsProps;
  list?: PreviewCommentList;
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
  drafts,
  list,
}: {
  collection: PreviewFeedbackCollectionController;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  closeFocus: (event: Event) => void;
  drafts?: PreviewUnsavedDraftsProps;
  list?: PreviewCommentList;
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
      <PreviewFeedbackCollection collection={collection} touch drafts={drafts} list={list} />
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
  side,
  portal,
  drafts,
  list,
}: {
  collection: PreviewFeedbackCollectionController;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  closeFocus: (event: Event) => void;
  trigger: ReactNode;
  side: "top" | "bottom";
  portal: boolean;
  drafts?: PreviewUnsavedDraftsProps;
  list?: PreviewCommentList;
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
          side={side}
          align="end"
          portal={portal}
          onCloseAutoFocus={closeFocus}
          onInteractOutside={keepOpenForCommentSurface}
          onEscapeKeyDown={keepOpenForCommentSurface}
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
            <PreviewFeedbackCollection
              collection={collection}
              touch={false}
              drafts={drafts}
              list={list}
            />
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
  trigger: customTrigger,
  side = "top",
  portal = false,
  drafts,
  list,
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
  const trigger = customTrigger ?? (
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
          drafts={drafts}
          list={list}
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
        side={side}
        portal={portal}
        drafts={drafts}
        list={list}
      />
    </>
  );
}
