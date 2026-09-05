"use client";

import { memo, useCallback, useEffect, useRef, useState } from "react";
import { IconCode } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { PanelHeaderBarSplit } from "@/components/task/panel-primitives";
import { FileViewerDownloadButton } from "@/components/task/file-viewer-header";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import { toRelativePath } from "@/lib/utils";
import {
  createPreviewRuntime,
  type PreviewRuntimeClient,
} from "@/lib/html-preview/preview-runtime";
import { PreviewSurface } from "@/lib/html-preview/preview-surface";
import {
  PreviewRuntimeError,
  type PreviewEvent,
  type PreviewRuntimeFailureCode,
  type PreviewSnapshot,
} from "@/lib/html-preview/preview-runtime-types";
import { useTranslation } from "react-i18next";

type HtmlPreviewContentProps = {
  path: string;
  content: string;
  worktreePath?: string;
  sessionId?: string;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
  showExternalVcsLink?: boolean;
  onDownload?: () => void;
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
}: Omit<HtmlPreviewContentProps, "content">) {
  const { t } = useTranslation();
  const fileStatus = useExternalVcsFileStatus(path, sessionId, repositoryName);

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
                className="h-11 w-11 cursor-pointer p-0 text-foreground sm:h-8 sm:w-8"
                data-testid="html-preview-code-toggle"
              >
                <IconCode className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("task:showCode")}</TooltipContent>
          </Tooltip>
        </div>
      }
    />
  );
}

export const HtmlPreviewContent = memo(function HtmlPreviewContent({
  path,
  content,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
  showExternalVcsLink = true,
  onDownload,
  onTogglePreview,
}: HtmlPreviewContentProps) {
  const { t } = useTranslation();
  const runtimeRef = useRef<PreviewRuntimeClient | null>(null);
  const generationRef = useRef(0);
  const [state, setState] = useState<
    | { status: "loading" }
    | { status: "ready"; snapshot: PreviewSnapshot }
    | { status: "failed"; code: PreviewRuntimeFailureCode }
  >({ status: "loading" });

  useEffect(() => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    const runtime = createPreviewRuntime();
    runtimeRef.current = runtime;
    setState({ status: "loading" });

    void runtime.load(content).then(
      (snapshot) => {
        if (generationRef.current === generation) setState({ status: "ready", snapshot });
      },
      (error: unknown) => {
        if (generationRef.current !== generation) return;
        const code = error instanceof PreviewRuntimeError ? error.code : "runtime-error";
        setState({ status: "failed", code });
      },
    );

    return () => {
      if (runtimeRef.current === runtime) runtimeRef.current = null;
      runtime.dispose();
    };
  }, [content]);

  const handleEvent = useCallback((event: PreviewEvent) => {
    const runtime = runtimeRef.current;
    const generation = generationRef.current;
    if (!runtime) return;
    void runtime.dispatch(event).then(
      (snapshot) => {
        if (generationRef.current === generation) setState({ status: "ready", snapshot });
      },
      (error: unknown) => {
        if (generationRef.current !== generation) return;
        const code = error instanceof PreviewRuntimeError ? error.code : "runtime-error";
        setState({ status: "failed", code });
      },
    );
  }, []);

  const failureMessage = getPreviewFailureMessage(
    state.status === "failed" ? state.code : "runtime-error",
    t,
  );

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
      <div className="min-h-0 flex-1 overflow-hidden">
        {state.status === "ready" && (
          <PreviewSurface snapshot={state.snapshot} onEvent={handleEvent} />
        )}
        {state.status === "loading" && (
          <div
            data-testid="html-preview-loading"
            className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground"
          >
            {t("task:htmlPreviewLoading")}
          </div>
        )}
        {state.status === "failed" && (
          <div
            data-testid="html-preview-error"
            className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground"
          >
            {failureMessage}
          </div>
        )}
      </div>
    </div>
  );
});

function getPreviewFailureMessage(
  code: PreviewRuntimeFailureCode,
  translate: (key: string) => string,
): string {
  if (code === "unsupported-capability") return translate("task:htmlPreviewUnsupportedCapability");
  if (code === "budget-exceeded") return translate("task:htmlPreviewBudgetExceeded");
  return translate("task:htmlPreviewRuntimeError");
}
