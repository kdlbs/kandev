"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  IconArrowLeft,
  IconCode,
  IconDeviceFloppy,
  IconEdit,
  IconEye,
  IconLoader2,
  IconRefresh,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PanelBody, PanelRoot } from "../panel-primitives";
import { FileViewerContent } from "../file-viewer-content";
import { HtmlPreviewContent } from "../html-preview-content";
import { MarkdownPreviewContent } from "../markdown-preview-content";
import { FileImageViewer } from "../file-image-viewer";
import { FileBinaryViewer } from "../file-binary-viewer";
import { FileViewerDownloadButton } from "../file-viewer-header";
import { HybridMarkdownEditor } from "@/components/editors/markdown/hybrid-markdown-editor";
import {
  getFileCategory,
  getFilePreviewKind,
  isMarkdownFile,
  type FilePreviewKind,
} from "@/lib/utils/file-types";
import { triggerFileDownload } from "@/lib/utils/file-download";
import { useAppStore } from "@/components/state-provider";
import type { OpenFileTab } from "@/lib/types/backend";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import {
  ExternalVcsFileLink,
  useExternalVcsFileStatus,
} from "@/components/editors/external-vcs-file-link";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateFileContent } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff } from "@/lib/utils/file-diff";
import {
  useHtmlPreviewPublisher,
  type HtmlPreviewPublishState,
} from "@/hooks/use-html-preview-publisher";
import {
  capitalize,
  isMarkdownFileModeSupported,
  type MarkdownFileMode,
} from "../markdown-file-mode";
import { getMobileFileIdentity, type MobileFileSavedSnapshot } from "./mobile-selected-file-state";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";
import { SymlinkIndicator } from "@/components/shared/symlink-indicator";

export type { MobileFileSavedSnapshot } from "./mobile-selected-file-state";

type MobileFileViewerPanelProps = {
  file: OpenFileTab;
  sessionId: string | null;
  onClose: () => void;
  /** Compatibility input for the previous rendered-preview state. */
  initialRenderedPreview?: boolean;
  /** Explicit mode from the mobile session selection state. */
  initialMarkdownMode?: MarkdownFileMode;
  /** Compatibility input for chat links that explicitly request Preview. */
  initialMarkdownPreview?: boolean;
  onFileChange?: (content: string) => void;
  onFileSaved?: (snapshot: MobileFileSavedSnapshot) => void;
  onModeChange?: (mode: MarkdownFileMode) => void;
  onReloadFromAgent?: () => void;
};

type ViewerKind = "image" | "binary" | "text";

const MARKDOWN_MODE_ORDER: readonly MarkdownFileMode[] = ["preview", "edit", "source"];

const MARKDOWN_MODE_ICONS = {
  preview: IconEye,
  edit: IconEdit,
  source: IconCode,
} as const;

function resolveViewerKind(file: OpenFileTab): ViewerKind {
  if (!file.isBinary) return "text";
  return getFileCategory(file.path) === "image" ? "image" : "binary";
}

function resolveInitialMarkdownMode(
  file: OpenFileTab,
  initialMarkdownMode?: MarkdownFileMode,
  initialMarkdownPreview?: boolean,
  initialRenderedPreview?: boolean,
): MarkdownFileMode | undefined {
  if (!isMarkdownFile(file.path)) return undefined;
  const requestedMode =
    file.markdownMode ??
    initialMarkdownMode ??
    (initialMarkdownPreview === true || initialRenderedPreview === true ? "preview" : "source");
  return isMarkdownFileModeSupported(file.path, requestedMode) ? requestedMode : "source";
}

function MobileMarkdownModeControls({
  path,
  mode,
  onModeChange,
}: {
  path: string;
  mode: MarkdownFileMode;
  onModeChange: (mode: MarkdownFileMode) => void;
}) {
  const { t } = useTranslation();
  const supportedModes = MARKDOWN_MODE_ORDER.filter((candidate) =>
    isMarkdownFileModeSupported(path, candidate),
  );

  return (
    <div
      className="flex min-w-0 shrink-0 items-center gap-1 overflow-x-auto overscroll-x-contain"
      role="group"
      aria-label={t("task:markdownModes")}
      data-testid="mobile-markdown-mode-controls"
    >
      {supportedModes.map((candidate) => {
        const Icon = MARKDOWN_MODE_ICONS[candidate];
        return (
          <Button
            key={candidate}
            type="button"
            variant={candidate === mode ? "secondary" : "ghost"}
            className="h-11 min-w-11 shrink-0 cursor-pointer gap-1 px-2 text-xs"
            data-testid={`mobile-markdown-mode-${candidate}`}
            aria-pressed={candidate === mode}
            onClick={() => onModeChange(candidate)}
          >
            <span className="inline-flex items-center gap-1">
              <Icon className="h-4 w-4" aria-hidden="true" />
              <span>{t(`task:markdownMode${capitalize(candidate)}`)}</span>
            </span>
          </Button>
        );
      })}
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function -- keeps the fixed mobile header and its touch actions together.
function MobileFileViewerHeader({
  file,
  fileStatus,
  activeTaskId,
  sessionId,
  repositoryId,
  markdownMode,
  viewerKind,
  previewKind,
  renderedPreview,
  htmlPreview,
  isDirty,
  isSaving,
  hasRemoteUpdate,
  onModeChange,
  onSave,
  onReloadFromAgent,
  onDownload,
  onPreviewHtml,
  onPreviewMarkdown,
  onClose,
}: {
  file: OpenFileTab;
  fileStatus: ReturnType<typeof useExternalVcsFileStatus>;
  activeTaskId: string | null;
  sessionId: string | null;
  repositoryId?: string;
  markdownMode?: MarkdownFileMode;
  viewerKind: ViewerKind;
  previewKind: FilePreviewKind;
  renderedPreview: boolean;
  htmlPreview: Pick<HtmlPreviewPublishState, "status" | "url" | "error"> & {
    isPublishing: boolean;
  };
  isDirty: boolean;
  isSaving: boolean;
  hasRemoteUpdate: boolean;
  onModeChange: (mode: MarkdownFileMode) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDownload?: () => void;
  onPreviewHtml: () => void;
  onPreviewMarkdown: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const canPreview =
    viewerKind === "text" && previewKind !== "none" && !renderedPreview && !markdownMode;
  const isHtml = previewKind === "html";
  const handlePreview = isHtml ? onPreviewHtml : onPreviewMarkdown;
  return (
    <div className="shrink-0 border-b border-border/80 bg-card/95 text-foreground">
      <div className="flex min-h-14 items-center gap-2 px-2">
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{file.path}</span>
        <SymlinkIndicator isSymlink={!!file.resolvedPath} showLabel />
        {isDirty && (
          <span
            className="shrink-0 text-primary"
            aria-label={t("common:unsavedChanges")}
            title={t("common:unsavedChanges")}
          >
            •
          </span>
        )}
        <ExternalVcsFileLink
          filePath={file.path}
          previousPath={fileStatus?.old_path}
          status={fileStatus?.status}
          taskId={activeTaskId}
          sessionId={sessionId}
          repositoryId={file.repo ? undefined : repositoryId}
          repositoryName={file.repo}
          size="touch"
        />
        <FileViewerDownloadButton onDownload={onDownload} />
        {canPreview && (
          <Button
            type="button"
            variant="ghost"
            className="h-11 min-w-11 shrink-0 cursor-pointer px-2"
            onClick={handlePreview}
            disabled={isHtml && htmlPreview.isPublishing}
            data-testid={isHtml ? "html-preview-toggle" : "markdown-preview-toggle"}
            aria-label={isHtml ? t("task:openHtmlPreview") : t("task:openMarkdownPreview")}
            title={isHtml ? t("task:htmlPreviewTrustedCode") : undefined}
          >
            {isHtml && htmlPreview.isPublishing ? (
              <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
            ) : (
              <IconEye className="h-4 w-4" aria-hidden="true" />
            )}
          </Button>
        )}
        <Button
          type="button"
          variant="ghost"
          className="h-11 min-w-11 shrink-0 cursor-pointer gap-1 px-2 text-xs"
          onClick={onClose}
          aria-label={t("common:back")}
        >
          <IconArrowLeft className="h-4 w-4" aria-hidden="true" />
          <span>{t("common:back")}</span>
        </Button>
      </div>
      <div className="flex min-h-14 min-w-0 items-center gap-2 overflow-hidden border-t border-border/60 px-2 pb-[env(safe-area-inset-bottom,0px)]">
        {markdownMode ? (
          <MobileMarkdownModeControls
            path={file.path}
            mode={markdownMode}
            onModeChange={onModeChange}
          />
        ) : (
          <div className="min-w-0 flex-1" />
        )}
        {hasRemoteUpdate && onReloadFromAgent && (
          <Button
            type="button"
            variant="outline"
            className="h-11 min-w-11 shrink-0 cursor-pointer gap-1 px-2 text-xs"
            onClick={onReloadFromAgent}
            aria-label={t("common:reload")}
            data-testid="mobile-file-reload"
          >
            <IconRefresh className="h-4 w-4" aria-hidden="true" />
            <span>{t("common:reload")}</span>
          </Button>
        )}
        <Button
          type="button"
          variant="default"
          className="h-11 min-w-11 shrink-0 cursor-pointer gap-1 px-3 text-xs"
          disabled={!isDirty || isSaving}
          onClick={onSave}
          data-testid="mobile-file-save"
        >
          {isSaving ? (
            <IconLoader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
          ) : (
            <IconDeviceFloppy className="h-4 w-4" aria-hidden="true" />
          )}
          <span>{isSaving ? t("task:saving") : t("common:save")}</span>
        </Button>
      </div>
    </div>
  );
}

function MobileViewerBody({
  file,
  viewerKind,
  previewKind,
  renderedPreview,
  markdownMode,
  keepHybridMounted,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  draftContent,
  baselineContent,
  onChange,
  onSourceFallback,
  onTogglePreview,
  onRetryHtmlPreview,
  htmlPreview,
}: {
  file: OpenFileTab;
  viewerKind: ViewerKind;
  previewKind: FilePreviewKind;
  renderedPreview: boolean;
  markdownMode?: MarkdownFileMode;
  keepHybridMounted: boolean;
  worktreePath?: string;
  sessionId: string | null;
  taskId: string | null;
  repositoryId?: string;
  draftContent: string;
  baselineContent: string;
  onChange: (content: string) => void;
  onSourceFallback?: () => void;
  onTogglePreview: () => void;
  onRetryHtmlPreview: () => void;
  htmlPreview: Pick<HtmlPreviewPublishState, "status" | "url" | "error"> & {
    isPublishing: boolean;
  };
}) {
  const markdownFile = isMarkdownFile(file.path);
  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="mobile-file-viewer-content">
      {viewerKind === "image" && (
        <FileImageViewer path={file.path} content={draftContent} worktreePath={worktreePath} />
      )}
      {viewerKind === "binary" && <FileBinaryViewer path={file.path} worktreePath={worktreePath} />}
      {viewerKind === "text" && markdownFile && (
        <MobileMarkdownSurface
          file={file}
          markdownMode={markdownMode}
          keepHybridMounted={keepHybridMounted}
          worktreePath={worktreePath}
          sessionId={sessionId}
          taskId={taskId}
          repositoryId={repositoryId}
          draftContent={draftContent}
          baselineContent={baselineContent}
          onChange={onChange}
          onSourceFallback={onSourceFallback}
        />
      )}
      {viewerKind === "text" && !markdownFile && renderedPreview && previewKind === "html" && (
        <HtmlPreviewContent
          path={file.path}
          worktreePath={worktreePath}
          sessionId={sessionId ?? undefined}
          taskId={taskId}
          repositoryId={repositoryId}
          repositoryName={file.repo}
          showExternalVcsLink={false}
          previewUrl={htmlPreview.url}
          isLoading={htmlPreview.isPublishing}
          error={htmlPreview.error}
          onRetry={onRetryHtmlPreview}
          onTogglePreview={onTogglePreview}
        />
      )}
      {viewerKind === "text" &&
        !markdownFile &&
        !(renderedPreview && previewKind === "html") && (
          <FileViewerContent
            path={file.path}
            repo={file.repo}
            content={draftContent}
            sessionId={sessionId ?? undefined}
            editable={false}
          />
        )}
    </div>
  );
}

function MobileMarkdownSurface({
  file,
  markdownMode,
  keepHybridMounted,
  worktreePath,
  sessionId,
  taskId,
  repositoryId,
  draftContent,
  baselineContent,
  onChange,
  onSourceFallback,
}: {
  file: OpenFileTab;
  markdownMode?: MarkdownFileMode;
  keepHybridMounted: boolean;
  worktreePath?: string;
  sessionId: string | null;
  taskId: string | null;
  repositoryId?: string;
  draftContent: string;
  baselineContent: string;
  onChange: (content: string) => void;
  onSourceFallback?: () => void;
}) {
  return (
    <>
      {markdownMode === "preview" && (
        <MarkdownPreviewContent
          path={file.path}
          content={draftContent}
          worktreePath={worktreePath}
          sessionId={sessionId ?? undefined}
          taskId={taskId}
          repositoryId={repositoryId}
          repositoryName={file.repo}
          enableComments={!!sessionId}
          showExternalVcsLink={false}
          onTogglePreview={undefined}
        />
      )}
      {keepHybridMounted && (
        <div
          className={markdownMode === "edit" ? "min-h-0 flex-1 overflow-hidden" : "hidden"}
          aria-hidden={markdownMode !== "edit"}
          data-testid="mobile-markdown-hybrid-editor-host"
        >
          <HybridMarkdownEditor
            content={draftContent}
            baseline={baselineContent}
            readOnly={false}
            onChange={onChange}
            onSourceFallback={onSourceFallback}
          />
        </div>
      )}
      {markdownMode === "source" && (
        <FileViewerContent
          path={file.path}
          repo={file.repo}
          content={draftContent}
          sessionId={sessionId ?? undefined}
          editable
          onChange={onChange}
        />
      )}
    </>
  );
}

function useMobileFileBuffer({
  file,
  initialMarkdownMode,
  initialMarkdownPreview,
  onFileChange,
  onModeChange,
}: {
  file: OpenFileTab;
  initialMarkdownMode?: MarkdownFileMode;
  initialMarkdownPreview?: boolean;
  onFileChange?: (content: string) => void;
  onModeChange?: (mode: MarkdownFileMode) => void;
}) {
  const fileIdentity = getMobileFileIdentity(file);
  const initialMode = resolveInitialMarkdownMode(file, initialMarkdownMode, initialMarkdownPreview);
  const [lastFileIdentity, setLastFileIdentity] = useState(fileIdentity);
  const fileIdentityRef = useRef(fileIdentity);
  fileIdentityRef.current = fileIdentity;
  const fileContentSnapshotRef = useRef({ identity: fileIdentity, content: file.content });
  const [markdownMode, setMarkdownMode] = useState<MarkdownFileMode | undefined>(initialMode);
  const [hybridMounted, setHybridMounted] = useState(initialMode === "edit");
  const [draftContent, setDraftContent] = useState(file.content);
  const [baselineContent, setBaselineContent] = useState(file.originalContent);
  const [originalHash, setOriginalHash] = useState(file.originalHash);

  if (lastFileIdentity !== fileIdentity) {
    setLastFileIdentity(fileIdentity);
    setMarkdownMode(initialMode);
    setHybridMounted(initialMode === "edit");
    setDraftContent(file.content);
    setBaselineContent(file.originalContent);
    setOriginalHash(file.originalHash);
  }

  useEffect(() => {
    const previous = fileContentSnapshotRef.current;
    const contentChanged = previous.identity === fileIdentity && previous.content !== file.content;
    if (contentChanged && !file.isDirty) {
      setDraftContent(file.content);
      setBaselineContent(file.originalContent);
      setOriginalHash(file.originalHash);
    }
    fileContentSnapshotRef.current = { identity: fileIdentity, content: file.content };
  }, [file.content, file.isDirty, file.originalContent, file.originalHash, fileIdentity]);

  const isDirty = draftContent !== baselineContent;
  const handleChange = useCallback(
    (content: string) => {
      setDraftContent(content);
      onFileChange?.(content);
    },
    [onFileChange],
  );
  const handleModeChange = useCallback(
    (mode: MarkdownFileMode) => {
      if (!isMarkdownFileModeSupported(file.path, mode)) return;
      if (mode === "edit") setHybridMounted(true);
      setMarkdownMode(mode);
      onModeChange?.(mode);
    },
    [file.path, onModeChange],
  );
  const handleSourceFallback = useCallback(() => {
    setMarkdownMode("source");
    onModeChange?.("source");
  }, [onModeChange]);
  const markSaved = useCallback((expectedFileIdentity: string, content: string, hash: string) => {
    if (fileIdentityRef.current !== expectedFileIdentity) return;
    setBaselineContent(content);
    setOriginalHash(hash);
  }, []);

  return {
    markdownMode,
    keepHybridMounted: hybridMounted && isMarkdownFileModeSupported(file.path, "edit"),
    draftContent,
    baselineContent,
    originalHash,
    isDirty,
    handleChange,
    handleModeChange,
    handleSourceFallback,
    markSaved,
  };
}

function useMobileFileSave({
  file,
  sessionId,
  fileIdentity,
  draftContent,
  baselineContent,
  originalHash,
  isDirty,
  markSaved,
  onFileSaved,
}: {
  file: OpenFileTab;
  sessionId: string | null;
  fileIdentity: string;
  draftContent: string;
  baselineContent: string;
  originalHash: string;
  isDirty: boolean;
  markSaved: (expectedFileIdentity: string, content: string, hash: string) => void;
  onFileSaved?: (snapshot: MobileFileSavedSnapshot) => void;
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [isSaving, setIsSaving] = useState(false);
  const handleSave = useCallback(async () => {
    if (!sessionId || !isDirty || isSaving) return;
    const client = getWebSocketClient();
    if (!client) return;
    setIsSaving(true);
    try {
      const contentToSave = draftContent;
      const response = await updateFileContent(client, sessionId, {
        path: file.path,
        diff: generateUnifiedDiff(baselineContent, contentToSave, file.path),
        originalHash,
        desiredContent: contentToSave,
        repo: file.repo,
      });
      if (response.success && response.new_hash) {
        markSaved(fileIdentity, contentToSave, response.new_hash);
        onFileSaved?.({
          path: file.path,
          repo: file.repo,
          sessionId,
          content: contentToSave,
          originalContent: contentToSave,
          originalHash: response.new_hash,
        });
        return;
      }
      toast({
        title: t("editors:saveFailed"),
        description: response.error || t("editors:failedToSaveFile"),
        variant: "error",
      });
    } catch (error) {
      toast({
        title: t("editors:saveFailed"),
        description: error instanceof Error ? error.message : t("editors:errorWhileSavingFile"),
        variant: "error",
      });
    } finally {
      setIsSaving(false);
    }
  }, [
    baselineContent,
    draftContent,
    fileIdentity,
    file.path,
    file.repo,
    isDirty,
    isSaving,
    markSaved,
    onFileSaved,
    originalHash,
    sessionId,
    t,
    toast,
  ]);

  return { isSaving, handleSave };
}

export function MobileFileViewerPanel({
  file,
  sessionId,
  onClose,
  initialRenderedPreview = false,
  initialMarkdownMode,
  initialMarkdownPreview,
  onFileChange,
  onFileSaved,
  onModeChange,
  onReloadFromAgent,
}: MobileFileViewerPanelProps) {
  const activeSession = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const worktreePath = getSessionWorkspacePath(activeSession);
  const repositoryId = activeSession?.repository_id ?? undefined;
  const fileStatus = useExternalVcsFileStatus(file.path, sessionId, file.repo);
  const viewerKind = useMemo(() => resolveViewerKind(file), [file.isBinary, file.path]);
  const previewKind = getFilePreviewKind(file.path, !!file.isBinary);
  const htmlPreview = useHtmlPreviewPublisher(sessionId);
  const onDownload = useMemo(
    () => () =>
      triggerFileDownload({
        fileName: file.path,
        content: file.content,
        isBinary: !!file.isBinary,
      }),
    [file.content, file.isBinary, file.path],
  );
  const fileIdentity = getMobileFileIdentity(file);
  const [lastFileIdentity, setLastFileIdentity] = useState(fileIdentity);
  const initialLegacyMarkdownPreview =
    initialMarkdownPreview || (initialRenderedPreview && previewKind === "markdown");
  const [renderedPreview, setRenderedPreview] = useState(
    false,
  );
  if (lastFileIdentity !== fileIdentity) {
    setLastFileIdentity(fileIdentity);
    setRenderedPreview(false);
  }
  useEffect(() => {
    htmlPreview.reset();
  }, [fileIdentity, htmlPreview.reset]);
  const publishCurrentHtmlPreview = useCallback(() => {
    setRenderedPreview(true);
    void htmlPreview.publish({ path: file.path, repo: file.repo, content: file.content });
  }, [file.content, file.path, file.repo, htmlPreview.publish]);
  const buffer = useMobileFileBuffer({
    file,
    initialMarkdownMode,
    initialMarkdownPreview: initialLegacyMarkdownPreview,
    onFileChange,
    onModeChange,
  });
  const save = useMobileFileSave({
    file,
    sessionId,
    fileIdentity: getMobileFileIdentity(file),
    draftContent: buffer.draftContent,
    baselineContent: buffer.baselineContent,
    originalHash: buffer.originalHash,
    isDirty: buffer.isDirty,
    markSaved: buffer.markSaved,
    onFileSaved,
  });
  const hasRemoteUpdate = file.hasRemoteUpdate ?? false;

  return (
    <PanelRoot data-testid="mobile-file-viewer-panel">
      <MobileFileViewerHeader
        file={file}
        fileStatus={fileStatus}
        activeTaskId={activeTaskId}
        sessionId={sessionId}
        repositoryId={repositoryId}
        markdownMode={buffer.markdownMode}
        viewerKind={viewerKind}
        previewKind={previewKind}
        renderedPreview={renderedPreview}
        htmlPreview={htmlPreview}
        isDirty={buffer.isDirty}
        isSaving={save.isSaving}
        hasRemoteUpdate={hasRemoteUpdate}
        onModeChange={buffer.handleModeChange}
        onSave={save.handleSave}
        onReloadFromAgent={onReloadFromAgent}
        onDownload={onDownload}
        onPreviewHtml={publishCurrentHtmlPreview}
        onPreviewMarkdown={() => setRenderedPreview(true)}
        onClose={onClose}
      />
      <PanelBody padding={false} scroll={false} className="overflow-hidden">
        <MobileViewerBody
          file={file}
          viewerKind={viewerKind}
          previewKind={previewKind}
          renderedPreview={renderedPreview}
          markdownMode={buffer.markdownMode}
          keepHybridMounted={buffer.keepHybridMounted}
          worktreePath={worktreePath}
          sessionId={sessionId}
          taskId={activeTaskId}
          repositoryId={repositoryId}
          draftContent={buffer.draftContent}
          baselineContent={buffer.baselineContent}
          onChange={buffer.handleChange}
          onSourceFallback={buffer.handleSourceFallback}
          onTogglePreview={() => setRenderedPreview((current) => !current)}
          onRetryHtmlPreview={publishCurrentHtmlPreview}
          htmlPreview={htmlPreview}
        />
      </PanelBody>
    </PanelRoot>
  );
}
