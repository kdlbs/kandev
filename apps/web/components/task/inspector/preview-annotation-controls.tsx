"use client";

import { useRef, type RefObject } from "react";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { PreviewCaptureController } from "@/hooks/use-preview-capture";
import { usePreviewToolbarPosition } from "@/hooks/use-preview-toolbar-position";
import { PreviewAnnotationToolbar } from "./preview-annotation-toolbar";
import { PreviewFeedbackCollectionSurface } from "./preview-feedback-collection";
import { previewCommentList } from "./preview-comment-list-model";

/**
 * The annotation toolbar over the preview frame. On fine pointers the user can
 * drag it anywhere over the frame; on coarse pointers it is the bottom bar,
 * and its panel toggle opens the comment sheet.
 */
export function PreviewAnnotationControls({
  capture,
  taskId,
  frame,
}: {
  capture: PreviewCaptureController;
  taskId: string | null | undefined;
  frame: RefObject<HTMLElement | null>;
}) {
  const touch = useTouchDrawer();
  const toolbarRef = useRef<HTMLDivElement>(null);
  const position = usePreviewToolbarPosition(frame, toolbarRef);
  return (
    <PreviewAnnotationToolbar
      ref={toolbarRef}
      touch={touch}
      position={touch ? undefined : position}
      tool={capture.annotation.tool}
      acknowledgedMode={capture.acknowledgedMode}
      pendingCount={capture.items.length + capture.drafts.length}
      regionDisabled={capture.limitReached}
      onToolChange={capture.selectTool}
      onExit={capture.annotation.exit}
      panelOpen={capture.commentPanel.open}
      onTogglePanel={capture.commentPanel.toggle}
      renderCount={(button) =>
        touch ? (
          <PreviewFeedbackCollectionSurface
            taskId={taskId}
            collection={capture}
            open={capture.commentPanel.open}
            onOpenChange={capture.commentPanel.setOpen}
            trigger={button}
            list={previewCommentList(capture)}
            drafts={{
              entries: capture.drafts,
              onResume: capture.openCapture,
              onDiscard: capture.discardDraft,
            }}
          />
        ) : (
          button
        )
      }
    />
  );
}
