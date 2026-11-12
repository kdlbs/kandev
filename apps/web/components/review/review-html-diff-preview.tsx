"use client";

import { useTranslation } from "react-i18next";
import type { HtmlPreviewPublishErrorCode } from "@/hooks/use-html-preview-publisher";
import { HtmlPreviewContent } from "@/components/task/html-preview-content";

export type ReviewHtmlDiffPreviewProps = {
  path: string;
  sessionId: string;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
  previewUrl: string | null;
  isLoading: boolean;
  error: HtmlPreviewPublishErrorCode | null;
  readError: boolean;
  onRefresh: () => void;
  onRetry: () => void;
  onShowDiff: () => void;
};

/**
 * Replaces one diff row's body with the complete current workspace file.
 * The source copy stays visible because the patch can come from a staged,
 * committed, or pull-request revision that differs from the workspace.
 */
export function ReviewHtmlDiffPreview({
  path,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
  previewUrl,
  isLoading,
  error,
  readError,
  onRefresh,
  onRetry,
  onShowDiff,
}: ReviewHtmlDiffPreviewProps) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="review-html-diff-preview"
      className="flex flex-col gap-2 border-b border-border/60 px-3 py-2"
    >
      <p data-testid="review-html-preview-source" className="text-xs text-muted-foreground">
        {t("review:htmlPreviewWorkspaceSource")}
      </p>
      <div className="h-[min(70dvh,36rem)] min-h-48 overflow-hidden rounded-md border border-border bg-background">
        <HtmlPreviewContent
          path={path}
          previewUrl={previewUrl}
          isLoading={isLoading}
          error={error}
          errorMessage={readError ? t("review:htmlPreviewReadFailed") : undefined}
          sessionId={sessionId}
          taskId={taskId}
          repositoryId={repositoryId}
          repositoryName={repositoryName}
          showExternalVcsLink={false}
          onRefresh={onRefresh}
          onRetry={onRetry}
          onTogglePreview={onShowDiff}
          toggleLabel={t("review:showDiff")}
        />
      </div>
    </div>
  );
}
