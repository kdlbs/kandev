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
import { MobileViewerBody, type MobileViewerKind } from "./mobile-markdown-viewer-body";
import { getFileCategory, isMarkdownFile } from "@/lib/utils/file-types";
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
import { useMarkdownEditorCommentState } from "../markdown-editor-comment-bridge";
import { useMarkdownFileLinkHandler } from "../markdown-file-link-handler";
import {
  capitalize,
  defaultMarkdownFileMode,
  isMarkdownFileModeSupported,
  type MarkdownFileMode,
} from "../markdown-file-mode";
import { getMobileFileIdentity, type MobileFileSavedSnapshot } from "./mobile-selected-file-state";
import { useToast } from "@/components/toast-provider";
import { useTranslation } from "react-i18next";

export type { MobileFileSavedSnapshot } from "./mobile-selected-file-state";

type MobileFileViewerPanelProps = {
  file: OpenFileTab;
  sessionId: string | null;
  onClose: () => void;
  /** Explicit mode from the mobile session selection state. */
  initialMarkdownMode?: MarkdownFileMode;
  /** Compatibility input for chat links that explicitly request Preview. */
  initialMarkdownPreview?: boolean;
  onFileChange?: (content: string) => void;
  onFileSaved?: (snapshot: MobileFileSavedSnapshot) => void;
  onModeChange?: (mode: MarkdownFileMode) => void;
  onReloadFromAgent?: () => void;
  onOpenFile?: (path: string, repo?: string) => void;
};

const MARKDOWN_MODE_ORDER: readonly MarkdownFileMode[] = ["preview", "edit", "source"];

const MARKDOWN_MODE_ICONS = {
  preview: IconEye,
  edit: IconEdit,
  source: IconCode,
} as const;

function resolveViewerKind(file: OpenFileTab): MobileViewerKind {
  if (!file.isBinary) return "text";
  return getFileCategory(file.path) === "image" ? "image" : "binary";
}

function resolveInitialMarkdownMode(
  file: OpenFileTab,
  initialMarkdownMode?: MarkdownFileMode,
  initialMarkdownPreview?: boolean,
): MarkdownFileMode | undefined {
  if (!isMarkdownFile(file.path)) return undefined;
  let legacyMode: MarkdownFileMode;
  if (initialMarkdownPreview === undefined) {
    legacyMode = defaultMarkdownFileMode(file.path) ?? "source";
  } else {
    legacyMode = initialMarkdownPreview ? "preview" : "source";
  }
  const requestedMode = file.markdownMode ?? initialMarkdownMode ?? legacyMode;
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
  isDirty,
  isSaving,
  hasRemoteUpdate,
  onModeChange,
  onSave,
  onReloadFromAgent,
  onClose,
}: {
  file: OpenFileTab;
  fileStatus: ReturnType<typeof useExternalVcsFileStatus>;
  activeTaskId: string | null;
  sessionId: string | null;
  repositoryId?: string;
  markdownMode?: MarkdownFileMode;
  isDirty: boolean;
  isSaving: boolean;
  hasRemoteUpdate: boolean;
  onModeChange: (mode: MarkdownFileMode) => void;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="shrink-0 border-b border-border/80 bg-card/95 text-foreground">
      <div className="flex min-h-14 items-center gap-2 px-2">
        <span className="min-w-0 flex-1 truncate font-mono text-xs">{file.path}</span>
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
  const [previewMounted, setPreviewMounted] = useState(initialMode === "preview");
  const [draftContent, setDraftContent] = useState(file.content);
  const [baselineContent, setBaselineContent] = useState(file.originalContent);
  const [originalHash, setOriginalHash] = useState(file.originalHash);

  if (lastFileIdentity !== fileIdentity) {
    setLastFileIdentity(fileIdentity);
    setMarkdownMode(initialMode);
    setHybridMounted(initialMode === "edit");
    setPreviewMounted(initialMode === "preview");
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
      if (mode === "preview") setPreviewMounted(true);
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
    keepPreviewMounted: previewMounted,
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

function useMobileFileViewerActions({
  file,
  worktreePath,
  markdownMode,
  onOpenFile,
  handleSave,
}: {
  file: OpenFileTab;
  worktreePath?: string;
  markdownMode?: MarkdownFileMode;
  onOpenFile?: (path: string, repo?: string) => void;
  handleSave: () => Promise<void>;
}) {
  const openLinkedFile = useCallback(
    (path: string) => onOpenFile?.(path, file.repo),
    [file.repo, onOpenFile],
  );
  const handleOpenLink = useMarkdownFileLinkHandler({
    path: file.path,
    worktreePath,
    onOpenFile: onOpenFile ? openLinkedFile : undefined,
  });
  const handleKeyDown = useCallback(
    (event: React.KeyboardEvent<HTMLDivElement>) => {
      if (markdownMode !== "edit" && markdownMode !== "source") return;
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "s") {
        event.preventDefault();
        void handleSave();
      }
    },
    [handleSave, markdownMode],
  );
  return { openLinkedFile, handleOpenLink, handleKeyDown };
}

export function MobileFileViewerPanel({
  file,
  sessionId,
  onClose,
  initialMarkdownMode,
  initialMarkdownPreview,
  onFileChange,
  onFileSaved,
  onModeChange,
  onReloadFromAgent,
  onOpenFile,
}: MobileFileViewerPanelProps) {
  const activeSession = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const worktreePath = getSessionWorkspacePath(activeSession);
  const repositoryId = activeSession?.repository_id ?? undefined;
  const fileStatus = useExternalVcsFileStatus(file.path, sessionId, file.repo);
  const viewerKind = useMemo(() => resolveViewerKind(file), [file.isBinary, file.path]);
  const buffer = useMobileFileBuffer({
    file,
    initialMarkdownMode,
    initialMarkdownPreview,
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
  const { openLinkedFile, handleOpenLink, handleKeyDown } = useMobileFileViewerActions({
    file,
    worktreePath,
    markdownMode: buffer.markdownMode,
    onOpenFile,
    handleSave: save.handleSave,
  });
  const { hybridComments, handleHybridComment } = useMarkdownEditorCommentState({
    path: file.path,
    content: buffer.draftContent,
    sessionId,
    repositoryId,
    enableComments: isMarkdownFile(file.path) && !!sessionId,
  });
  const hasRemoteUpdate = file.hasRemoteUpdate ?? false;

  return (
    <PanelRoot data-testid="mobile-file-viewer-panel" onKeyDown={handleKeyDown}>
      <MobileFileViewerHeader
        file={file}
        fileStatus={fileStatus}
        activeTaskId={activeTaskId}
        sessionId={sessionId}
        repositoryId={repositoryId}
        markdownMode={buffer.markdownMode}
        isDirty={buffer.isDirty}
        isSaving={save.isSaving}
        hasRemoteUpdate={hasRemoteUpdate}
        onModeChange={buffer.handleModeChange}
        onSave={save.handleSave}
        onReloadFromAgent={onReloadFromAgent}
        onClose={onClose}
      />
      <PanelBody padding={false} scroll={false} className="overflow-hidden">
        <MobileViewerBody
          file={file}
          viewerKind={viewerKind}
          markdownMode={buffer.markdownMode}
          keepHybridMounted={buffer.keepHybridMounted}
          keepPreviewMounted={buffer.keepPreviewMounted}
          worktreePath={worktreePath}
          sessionId={sessionId}
          taskId={activeTaskId}
          repositoryId={repositoryId}
          draftContent={buffer.draftContent}
          comments={hybridComments}
          onChange={buffer.handleChange}
          onComment={handleHybridComment}
          onSourceFallback={buffer.handleSourceFallback}
          onOpenFile={onOpenFile ? openLinkedFile : undefined}
          onOpenLink={onOpenFile ? handleOpenLink : undefined}
        />
      </PanelBody>
    </PanelRoot>
  );
}
