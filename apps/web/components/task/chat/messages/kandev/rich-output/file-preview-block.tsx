"use client";

import { useCallback, useId, useState } from "react";
import { IconChevronDown, IconChevronRight, IconExternalLink, IconFile } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { GridSpinner } from "@/components/grid-spinner";
import { getFileCategory, getImageMimeType } from "@/lib/utils/file-types";
import {
  useWorkspaceFilePreview,
  type WorkspaceFilePreviewState,
} from "@/hooks/domains/session/use-workspace-file-preview";
import type { RichOutputFileBlock } from "./types";

const TEXT_PREVIEW_CHARS = 2000;

function FilePreviewContent({
  block,
  state,
}: {
  block: RichOutputFileBlock;
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
    if (getFileCategory(block.path) !== "image") {
      return (
        <p className="py-4 text-xs text-muted-foreground">
          {t("task:richOutputBinaryPreviewUnavailable")}
        </p>
      );
    }
    return (
      <div className="flex justify-center py-2">
        <img
          src={`data:${getImageMimeType(block.path)};base64,${state.response.content}`}
          alt={block.title || block.path}
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

type FilePreviewBlockProps = {
  block: RichOutputFileBlock;
  sessionId?: string;
  onOpenFile?: (path: string, repo?: string) => void;
};

export function FilePreviewBlock({ block, sessionId, onOpenFile }: FilePreviewBlockProps) {
  const { t } = useTranslation();
  const previewId = useId();
  const [expanded, setExpanded] = useState(false);
  const { load, state } = useWorkspaceFilePreview(sessionId, block.path, block.repo);

  const handleToggle = useCallback(() => {
    const next = !expanded;
    setExpanded(next);
    if (next && (state.kind === "idle" || state.kind === "error")) void load();
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
            {block.title && <p className="truncate text-xs font-medium">{block.title}</p>}
            <p className="truncate font-mono text-[11px] text-muted-foreground" title={block.path}>
              {block.repo ? `${block.repo}:` : ""}
              {block.path}
            </p>
            {block.caption && (
              <p className="mt-0.5 text-[11px] leading-relaxed text-muted-foreground">
                {block.caption}
              </p>
            )}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1 self-end min-[420px]:self-auto">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="min-h-11 cursor-pointer gap-1.5 px-2.5 min-[640px]:min-h-9"
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
              size="sm"
              className="min-h-11 cursor-pointer gap-1.5 px-2.5 min-[640px]:min-h-9"
              onClick={() => onOpenFile(block.path, block.repo)}
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
          <FilePreviewContent block={block} state={state} />
        </div>
      )}
    </article>
  );
}
