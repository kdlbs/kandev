"use client";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useFileOperations } from "@/hooks/use-file-operations";
import type { usePreviewAsText } from "@/hooks/use-preview-as-text";
import type { fileViewerCandidates } from "@/lib/plugins/file-viewer-selection";
import { FileViewerDownloadButton, FileViewerExternalLink } from "./file-viewer-header";

export function FilePreviewToolbar({
  path,
  repo,
  sessionId,
  candidates,
  choice,
  viewerAvailable,
  text,
  onSelect,
  onOpenText,
}: {
  path: string;
  repo?: string;
  sessionId: string;
  candidates: ReturnType<typeof fileViewerCandidates>;
  choice: string | null;
  viewerAvailable: boolean;
  text: ReturnType<typeof usePreviewAsText>;
  onSelect: (value: string) => void;
  onOpenText?: () => void;
}) {
  const { t } = useTranslation();
  const { downloadFile } = useFileOperations(sessionId);
  const showChoice =
    candidates.length > 0 &&
    (candidates.length > 1 || !viewerAvailable || !!text.file || text.loading);
  return (
    <div className="flex flex-wrap items-center gap-2 border-b p-2">
      <span className="min-w-0 flex-1 break-all text-sm">{path}</span>
      <FileViewerExternalLink path={path} sessionId={sessionId} repositoryName={repo} />
      <FileViewerDownloadButton onDownload={() => void downloadFile(path, repo)} />
      {showChoice && (
        <Select value={text.file || text.loading ? "" : (choice ?? "")} onValueChange={onSelect}>
          <SelectTrigger aria-label={t("plugins:viewerOpenWith")} className="w-auto">
            <SelectValue placeholder={t("plugins:viewerOpenWith")} />
          </SelectTrigger>
          <SelectContent>
            {candidates.map((v) => (
              <SelectItem key={`${v.pluginId}/${v.id}`} value={`${v.pluginId}/${v.id}`}>
                {v.title}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      {
        <Button
          className="max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          variant="outline"
          disabled={text.loading}
          onClick={onOpenText ?? (() => void text.load())}
        >
          {t("plugins:viewerOpenText")}
        </Button>
      }
    </div>
  );
}
