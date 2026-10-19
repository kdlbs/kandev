"use client";

import { useCallback, useId, useEffect } from "react";
import { IconChevronDown, IconChevronRight, IconExternalLink, IconFile } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";
import { getFileCategory, getImageMimeType } from "@/lib/utils/file-types";
import {
  useWorkspaceFilePreview,
  type WorkspaceFilePreviewState,
} from "@/hooks/domains/session/use-workspace-file-preview";
import type { FilePreviewProps } from "@/lib/rich-output/catalog";

import { useRichOutputViewState } from "./view-state";

const TEXT_PREVIEW_CHARS = 2000;

function FilePreviewContent({
  file,
  state,
}: {
  file: FilePreviewProps;
  state: WorkspaceFilePreviewState;
}) {
  const { t } = useTranslation();
  if (state.kind === "loading") {
    return (
      <div className="flex min-h-20 items-center justify-center gap-2 text-xs text-muted-foreground">
        <GridSpinner />
        <span>{t("task:richOutputLoadingPreview")}</span>
      </div>
    );
  }
  if (state.kind === "error" || state.kind === "idle") {
    return (
      <p className="py-4 text-xs text-muted-foreground">{t("task:richOutputPreviewFailed")}</p>
    );
  }
  if (state.response.is_binary) {
    if (getFileCategory(file.path) !== "image") {
      return (
        <p className="py-4 text-xs text-muted-foreground">
          {t("task:richOutputBinaryPreviewUnavailable")}
        </p>
      );
    }
    return (
      <div className="flex justify-center py-2">
        <img
          src={`data:${getImageMimeType(file.path)};base64,${state.response.content}`}
          alt={file.title || file.path}
          className="max-h-64 max-w-full rounded-md object-contain ring-1 ring-black/10 dark:ring-white/10"
          draggable={false}
        />
      </div>
    );
  }
  const clipped = state.response.content.slice(0, TEXT_PREVIEW_CHARS);
  const wasClipped = state.response.content.length > TEXT_PREVIEW_CHARS;
  return (
    <div className="space-y-2 py-2">
      <pre className="max-h-52 max-w-full overflow-y-auto whitespace-pre-wrap break-words font-mono text-[10px] leading-relaxed text-foreground">
        {clipped}
      </pre>
      {wasClipped && (
        <p className="text-[10px] text-muted-foreground">{t("task:richOutputPreviewTruncated")}</p>
      )}
    </div>
  );
}

type NativeFilePreviewProps = FilePreviewProps & {
  sessionId?: string;
  onOpenFile?: (path: string, repo?: string) => void;
};

export function FilePreview({ sessionId, onOpenFile, ...file }: NativeFilePreviewProps) {
  const { t } = useTranslation();
  const previewId = useId();
  const [expanded, setExpanded] = useRichOutputViewState(
    JSON.stringify([file.repo, file.path]),
    getFileCategory(file.path) === "image",
  );
  const { load, state } = useWorkspaceFilePreview(sessionId, file.path, file.repo);

  useEffect(() => {
    if (expanded && state.kind === "idle") void load();
  }, [expanded, load, state.kind]);

  const handleToggle = useCallback(() => {
    const next = !expanded;
    setExpanded(next);
    if (next && state.kind === "error") void load();
  }, [expanded, load, state.kind]);

  return (
    <article
      className="min-w-0 rounded-lg border border-border/50 bg-background/50"
      data-testid="rich-output-file"
    >
      <div className="flex min-w-0 flex-col gap-2 px-3 py-2 min-[420px]:flex-row min-[420px]:items-center">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <IconFile className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          <div className="min-w-0">
            {file.title && <p className="truncate text-xs font-medium">{file.title}</p>}
            <p className="truncate font-mono text-[11px] text-muted-foreground" title={file.path}>
              {file.repo ? `${file.repo}:` : ""}
              {file.path}
            </p>
            {file.caption && (
              <p className="mt-0.5 text-[11px] leading-relaxed text-muted-foreground">
                {file.caption}
              </p>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1 self-end min-[420px]:self-auto">
          <Button
            type="button"
            variant="ghost"
            className="cursor-pointer gap-1.5 px-2.5 max-md:h-12 [@media(pointer:coarse)]:h-12"
            aria-expanded={expanded}
            aria-controls={previewId}
            onClick={handleToggle}
            data-testid="rich-output-file-preview-toggle"
          >
            {expanded ? (
              <IconChevronDown className="size-4" />
            ) : (
              <IconChevronRight className="size-4" />
            )}
            {expanded ? t("task:richOutputHidePreview") : t("task:richOutputPreview")}
          </Button>
          {onOpenFile && (
            <Button
              type="button"
              variant="ghost"
              className="cursor-pointer gap-1.5 px-2.5 max-md:h-12 [@media(pointer:coarse)]:h-12"
              onClick={() => onOpenFile(file.path, file.repo)}
              data-testid="rich-output-file-open"
            >
              <IconExternalLink className="size-4" />
              {t("task:richOutputOpenFile")}
            </Button>
          )}
        </div>
      </div>
      {expanded && (
        <div id={previewId} className="border-t border-border/50 px-3 py-2">
          <FilePreviewContent file={file} state={state} />
        </div>
      )}
    </article>
  );
}
