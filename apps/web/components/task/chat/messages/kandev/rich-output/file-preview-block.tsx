"use client";

import { useCallback, useId, useMemo, useEffect, useState } from "react";
import { IconChevronDown, IconChevronRight, IconExternalLink, IconFile } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useOptionalAppStore } from "@/components/state-provider";
import { GridSpinner } from "@/components/grid-spinner";
import { ImagePreviewDialog } from "@/components/task/chat/image-preview-dialog";
import { getFileCategory, getImageMimeType } from "@/lib/utils/file-types";
import {
  useWorkspaceFilePreview,
  type WorkspaceFilePreviewState,
} from "@/hooks/domains/session/use-workspace-file-preview";
import type { FilePreviewProps } from "@/lib/rich-output/catalog";

import { usePluginRegistry } from "@/lib/plugins/registry";
import { shouldOpenFilePreview, fileViewerCandidates } from "@/lib/plugins/file-viewer-selection";
import { PluginFilePreview } from "@/components/task/plugin-file-preview";

import { useRichOutputViewState } from "./view-state";

import { parsePreviewDiff } from "./diff-preview-model";
import { DiffPreview } from "./diff-preview";

const DIFF_PATH = /\.(diff|patch)$/i;
const TEXT_PREVIEW_CHARS = 2000;

function FilePreviewContent({
  file,
  state,
}: {
  file: FilePreviewProps;
  state: WorkspaceFilePreviewState;
}) {
  const { t } = useTranslation();
  const content =
    "response" in state && !state.response.is_binary ? state.response.content : undefined;
  const diff = useMemo(
    () => (content && DIFF_PATH.test(file.path) ? parsePreviewDiff(content) : null),
    [content, file.path],
  );
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
        <ImagePreviewDialog
          src={`data:${getImageMimeType(file.path)};base64,${state.response.content}`}
          alt={file.title || file.path}
          thumbnailClassName="max-h-64 max-w-full rounded-md object-contain ring-1 ring-black/10 dark:ring-white/10"
        />
      </div>
    );
  }
  if (diff) return <DiffPreview diff={diff} />;
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

function expandByDefault(path: string, pluginPreview: boolean, expandImages: boolean) {
  if (pluginPreview) return fileViewerCandidates(path).length > 0;
  return DIFF_PATH.test(path) || (getFileCategory(path) === "image" && expandImages);
}

export function FilePreview({ sessionId, onOpenFile, ...file }: NativeFilePreviewProps) {
  const { t } = useTranslation();
  const previewId = useId();
  usePluginRegistry();
  const eligible = shouldOpenFilePreview(file.path);
  // Keep the permission-aware route if a plugin disappears while this card is open.
  const [pluginPreview, setPluginPreview] = useState(eligible);
  if (eligible && !pluginPreview) setPluginPreview(true);
  const fileCategory = getFileCategory(file.path);
  const defaultExpanded = useOptionalAppStore(
    (store) => store.imagePreviewsExpandedByDefault,
    true,
  );
  const [expanded, setExpanded] = useRichOutputViewState(
    JSON.stringify([file.repo, file.path]),
    expandByDefault(file.path, pluginPreview, defaultExpanded),
  );
  const { load, state } = useWorkspaceFilePreview(sessionId, file.path, file.repo);

  useEffect(() => {
    if (!pluginPreview && expanded && state.kind === "idle") void load();
  }, [pluginPreview, expanded, load, state.kind]);

  const handleToggle = useCallback(() => {
    const next = !expanded;
    if (!pluginPreview && next && (fileCategory !== "image" || state.kind === "error")) {
      void load();
    }
    setExpanded(next);
  }, [pluginPreview, expanded, fileCategory, state.kind, load]);

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
          {pluginPreview && sessionId ? (
            <div
              className="h-[min(32rem,70dvh)] min-h-80 overflow-hidden"
              data-testid="rich-output-plugin-viewport"
            >
              <PluginFilePreview path={file.path} repo={file.repo} sessionId={sessionId} />
            </div>
          ) : (
            <FilePreviewContent file={file} state={state} />
          )}
        </div>
      )}
    </article>
  );
}
