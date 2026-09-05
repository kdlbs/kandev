"use client";

import { memo } from "react";
import { IconCode } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { FileViewerDownloadButton } from "@/components/task/file-viewer-header";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import {
  getHtmlPreviewPublishErrorKey,
  type HtmlPreviewPublishErrorCode,
} from "@/hooks/use-html-preview-publisher";
import { toRelativePath } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type HtmlPreviewContentProps = {
  path: string;
  previewUrl?: string | null;
  isLoading?: boolean;
  error?: HtmlPreviewPublishErrorCode | null;
  worktreePath?: string;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
  showExternalVcsLink?: boolean;
  onDownload?: () => void;
  onRetry?: () => void;
  onTogglePreview: () => void;
};

function HtmlPreviewContentToolbar({
  path,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
  showExternalVcsLink,
  onDownload,
  onTogglePreview,
}: Omit<HtmlPreviewContentProps, "previewUrl" | "isLoading" | "error" | "onRetry">) {
  const { t } = useTranslation();
  const fileStatus = useExternalVcsFileStatus(path, sessionId, repositoryName);
  const trustWarning = t("task:htmlPreviewTrustedCode");

  return (
    <PanelHeaderBarSplit
      left={
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          <span className="truncate font-mono">{toRelativePath(path, worktreePath)}</span>
          <span className="shrink-0 text-xs text-muted-foreground/60">{t("task:htmlPreview")}</span>
        </div>
      }
      right={
        <div className="flex items-center gap-1">
          {showExternalVcsLink && (
            <ExternalVcsFileLink
              filePath={path}
              previousPath={fileStatus?.old_path}
              status={fileStatus?.status}
              taskId={taskId}
              sessionId={sessionId}
              repositoryId={repositoryName ? undefined : repositoryId}
              repositoryName={repositoryName}
              size="sm"
            />
          )}
          <FileViewerDownloadButton onDownload={onDownload} />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                size="sm"
                variant="ghost"
                onClick={onTogglePreview}
                aria-label={t("task:showCode")}
                title={trustWarning}
                className="h-11 w-11 cursor-pointer p-0 text-foreground sm:h-8 sm:w-8"
                data-testid="html-preview-code-toggle"
              >
                <IconCode className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              <p>{t("task:showCode")}</p>
              <p className="mt-1 max-w-xs text-muted-foreground">{trustWarning}</p>
            </TooltipContent>
          </Tooltip>
        </div>
      }
    />
  );
}

export const HtmlPreviewContent = memo(function HtmlPreviewContent({
  path,
  previewUrl,
  isLoading = false,
  error = null,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
  showExternalVcsLink = true,
  onDownload,
  onRetry,
  onTogglePreview,
}: HtmlPreviewContentProps) {
  const { t } = useTranslation();
  const errorKey = error ? getHtmlPreviewPublishErrorKey(error) : null;

  return (
    <div className="relative flex h-full min-h-0 flex-col" data-testid="html-preview">
      <HtmlPreviewContentToolbar
        path={path}
        worktreePath={worktreePath}
        sessionId={sessionId}
        taskId={taskId}
        repositoryId={repositoryId}
        repositoryName={repositoryName}
        showExternalVcsLink={showExternalVcsLink}
        onDownload={onDownload}
        onTogglePreview={onTogglePreview}
      />
      <p
        data-testid="html-preview-trust-warning"
        className="shrink-0 border-b border-border/70 bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground"
      >
        {t("task:htmlPreviewTrustedCode")}
      </p>
      <div className="min-h-0 flex-1 overflow-hidden">
        {isLoading && (
          <div
            data-testid="html-preview-loading"
            className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground"
          >
            {t("task:htmlPreviewLoading")}
          </div>
        )}
        {!isLoading && errorKey && (
          <div
            data-testid="html-preview-error"
            role="alert"
            className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm text-muted-foreground"
          >
            <p>{t(errorKey)}</p>
            {onRetry && (
              <Button type="button" variant="outline" onClick={onRetry}>
                {t("task:retryHtmlPreview")}
              </Button>
            )}
          </div>
        )}
        {!isLoading && !errorKey && previewUrl && (
          <iframe
            data-testid="html-preview-frame"
            src={previewUrl}
            title={t("task:browserPreview")}
            className="h-full w-full border-0"
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
            referrerPolicy="no-referrer"
          />
        )}
      </div>
    </div>
  );
});
