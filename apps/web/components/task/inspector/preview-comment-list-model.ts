import type { PreviewCaptureController } from "@/hooks/use-preview-capture";
import type { PreviewCommentList } from "./preview-comment-list";

/** Shares marker numbering, selection, and editor actions across annotation surfaces. */
export function previewCommentList(capture: PreviewCaptureController): PreviewCommentList {
  return {
    numbers: capture.captureNumbers,
    activeId: capture.openDraft?.id ?? capture.editingItem?.id ?? null,
    selectedId: capture.selectedId,
    onOpen: (id) => {
      capture.selectCapture(id);
      capture.openCapture(id);
    },
    onSelect: capture.selectCapture,
  };
}
