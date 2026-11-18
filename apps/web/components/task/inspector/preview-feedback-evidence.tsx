"use client";

import { useTranslation } from "react-i18next";
import { attachmentContentUrl } from "@/lib/api/domains/attachment-api";
import type { PreviewFeedbackDraft } from "@/hooks/use-preview-capture";
import type { PreviewElementSnapshot, TaskPreviewFeedback } from "@/lib/types/http";

/** The evidence a feedback comment is about, independent of draft or saved state. */
export type PreviewFeedbackEvidenceValue =
  | { kind: "text"; text: string }
  | { kind: "element"; text: string }
  | {
      kind: "screenshot";
      /** `null` while the region is still being captured. */
      imageUrl: string | null;
      caption: string;
      uniformOverCanvas?: boolean;
    };

export function previewFeedbackElementLabel(item: {
  element_snapshot?: PreviewElementSnapshot | null;
}) {
  const element = item.element_snapshot;
  if (!element) return "";
  let label = element.tag;
  if (element.id) label += `#${element.id}`;
  else if (element.classes[0]) label += `.${element.classes[0]}`;
  return `<${label}>`;
}

export function evidenceForItem(item: TaskPreviewFeedback): PreviewFeedbackEvidenceValue {
  if (item.kind === "screenshot" && item.screenshot_attachment) {
    return {
      kind: "screenshot",
      imageUrl: attachmentContentUrl(item.screenshot_attachment.attachment_id),
      caption: item.screenshot_attachment.name,
    };
  }
  if (item.kind === "element") return { kind: "element", text: previewFeedbackElementLabel(item) };
  return { kind: "text", text: item.selected_text ?? item.screenshot_attachment?.name ?? "" };
}

/** `dimensions` is the localized "W × H · PNG" caption for an unsaved screenshot. */
export function evidenceForDraft(
  draft: PreviewFeedbackDraft,
  dimensions: string,
): PreviewFeedbackEvidenceValue {
  if (draft.kind === "screenshot") {
    return {
      kind: "screenshot",
      imageUrl: draft.screenshot?.previewUrl ?? null,
      caption: dimensions,
      uniformOverCanvas: draft.screenshot?.uniformOverCanvas,
    };
  }
  if (draft.kind === "element")
    return { kind: "element", text: previewFeedbackElementLabel(draft) };
  return { kind: "text", text: draft.selected_text ?? "" };
}

/** Plain-text form for quoting the evidence, such as a Drawer description. */
export function evidenceText(evidence: PreviewFeedbackEvidenceValue) {
  return evidence.kind === "screenshot" ? evidence.caption : evidence.text;
}

export function PreviewFeedbackEvidence({ evidence }: { evidence: PreviewFeedbackEvidenceValue }) {
  const { t } = useTranslation();
  if (evidence.kind !== "screenshot") {
    return <p className="line-clamp-3 break-words font-mono text-xs">{evidence.text}</p>;
  }
  return (
    <div className="space-y-1">
      {evidence.imageUrl ? (
        <img
          src={evidence.imageUrl}
          alt={t("task:previewScreenshotAlt")}
          className="max-h-40 w-full rounded border bg-background object-contain"
        />
      ) : (
        <p
          role="status"
          className="flex h-20 w-full animate-pulse items-center justify-center rounded border bg-muted text-xs text-muted-foreground"
          data-testid="preview-feedback-evidence-capturing"
        >
          {t("task:previewRasterizing")}
        </p>
      )}
      <p className="truncate font-mono text-xs text-muted-foreground">{evidence.caption}</p>
      {evidence.uniformOverCanvas && (
        <p role="status" className="text-xs text-amber-600 dark:text-amber-400">
          {t("task:previewScreenshotCanvasUniformWarning")}
        </p>
      )}
    </div>
  );
}
