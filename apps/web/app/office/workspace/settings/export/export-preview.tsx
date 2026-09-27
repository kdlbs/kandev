"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconArrowLeft, IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import * as officeApi from "@/lib/api/domains/office-api";
import { triggerBlobDownload } from "@/lib/utils/file-download";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { ExportFileTree } from "./export-file-tree";
import { ExportFilePreview } from "./export-file-preview";
import { buildFileTree, countSelectedFiles } from "./export-utils";
import type { ExportFile } from "./export-types";
import { useTranslation } from "react-i18next";

type ExportToolbarProps = {
  workspaceName: string;
  selectedCount: number;
  total: number;
  onExport: () => void;
};

function ExportToolbar({ workspaceName, selectedCount, total, onExport }: ExportToolbarProps) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
      <span className="text-sm text-muted-foreground">
        {t("office:exportSelectionSummary", {
          workspace: workspaceName,
          selected: selectedCount,
          total,
        })}
      </span>
      <Button
        size="sm"
        onClick={onExport}
        disabled={selectedCount === 0}
        className="cursor-pointer"
      >
        <IconDownload className="h-4 w-4 mr-1.5" />
        {t("office:exportFilesCount", { count: selectedCount })}
      </Button>
    </div>
  );
}

type ExportErrorProps = {
  errorKey: string;
  onRetry: () => void;
};

function ExportDownloadError({ errorKey, onRetry }: ExportErrorProps) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center justify-between gap-3 border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-sm text-destructive">
      <span>{t(errorKey)}</span>
      {errorKey === "office:exportConfigurationChanged" && (
        <Button size="sm" variant="ghost" onClick={onRetry} className="min-h-11 cursor-pointer">
          {t("office:retry")}
        </Button>
      )}
    </div>
  );
}

type ExportContentProps = {
  usesTouchDrawer: boolean;
  previewFile: ExportFile | null;
  tree: ReturnType<typeof buildFileTree>;
  selectedPaths: Set<string>;
  onSelectedPathsChange: (paths: Set<string>) => void;
  previewPath: string | null;
  onPreviewPathChange: (path: string | null) => void;
};

function ExportContent({
  usesTouchDrawer,
  previewFile,
  tree,
  selectedPaths,
  onSelectedPathsChange,
  previewPath,
  onPreviewPathChange,
}: ExportContentProps) {
  const { t } = useTranslation();
  if (usesTouchDrawer && previewFile) {
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <Button
          variant="ghost"
          className="min-h-11 shrink-0 justify-start gap-2 rounded-none border-b px-4 cursor-pointer"
          onClick={() => onPreviewPathChange(null)}
        >
          <IconArrowLeft className="h-4 w-4" />
          {t("office:backToExportFiles")}
        </Button>
        <ExportFilePreview file={previewFile} />
      </div>
    );
  }

  return (
    <>
      <ExportFileTree
        tree={tree}
        selectedPaths={selectedPaths}
        onSelectedPathsChange={onSelectedPathsChange}
        previewPath={previewPath}
        onPreviewPathChange={(path) => onPreviewPathChange(path)}
      />
      {!usesTouchDrawer && <ExportFilePreview file={previewFile} />}
    </>
  );
}

type ExportStatusProps = {
  activeWorkspaceId: string;
  loading: boolean;
  errorKey: string | null;
  onRetry: () => void;
};

function ExportStatus({
  activeWorkspaceId,
  loading,
  errorKey,
  onRetry,
}: ExportStatusProps): React.ReactNode {
  const { t } = useTranslation();
  if (!activeWorkspaceId) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        {t("office:selectAWorkspaceToExport")}
      </div>
    );
  }
  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        {t("office:loadingExportBundle")}
      </div>
    );
  }
  if (errorKey) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-destructive">
        <span>{t(errorKey)}</span>
        <Button variant="outline" onClick={onRetry} className="cursor-pointer">
          {t("office:retry")}
        </Button>
      </div>
    );
  }
  return null;
}

function useExportManifest(
  activeWorkspaceId: string,
  usesTouchDrawer: boolean,
  reloadToken: number,
) {
  const [files, setFiles] = useState<ExportFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [previewPath, setPreviewPath] = useState<string | null>(null);
  const [revision, setRevision] = useState("");

  useEffect(() => {
    setErrorKey(null);
    setFiles([]);
    setSelectedPaths(new Set());
    setPreviewPath(null);
    setRevision("");
    if (!activeWorkspaceId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    let cancelled = false;
    officeApi
      .exportConfigManifest(activeWorkspaceId)
      .then((manifest) => {
        if (cancelled) return;
        setFiles(manifest.files);
        setRevision(manifest.revision);
        setSelectedPaths(new Set(manifest.files.map((file) => file.path)));
        setPreviewPath(usesTouchDrawer ? null : (manifest.files[0]?.path ?? null));
      })
      .catch(() => {
        if (!cancelled) setErrorKey("office:failedToLoadExportBundle");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [activeWorkspaceId, reloadToken, usesTouchDrawer]);

  return {
    files,
    loading,
    errorKey,
    selectedPaths,
    previewPath,
    revision,
    setSelectedPaths,
    setPreviewPath,
  };
}

function useWorkspaceDownloadGeneration(activeWorkspaceId: string) {
  const workspaceGeneration = useRef(0);
  const lastWorkspaceId = useRef(activeWorkspaceId);
  if (lastWorkspaceId.current !== activeWorkspaceId) {
    lastWorkspaceId.current = activeWorkspaceId;
    workspaceGeneration.current += 1;
  }
  return { workspaceGeneration, lastWorkspaceId };
}

export function ExportPreview() {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const workspaces = useAppStore((s) => s.workspaces);
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  // Fallback LABEL for the bundle header, shown only before the workspace loads.
  const workspaceName = activeWorkspace?.name || t("common:workspace");

  const [reloadToken, setReloadToken] = useState(0);
  const [downloadErrorKey, setDownloadErrorKey] = useState<string | null>(null);
  const { workspaceGeneration, lastWorkspaceId } =
    useWorkspaceDownloadGeneration(activeWorkspaceId);
  const usesTouchDrawer = useTouchDrawer();
  const {
    files,
    loading,
    errorKey,
    selectedPaths,
    previewPath,
    revision,
    setSelectedPaths,
    setPreviewPath,
  } = useExportManifest(activeWorkspaceId, usesTouchDrawer, reloadToken);

  useEffect(() => {
    setDownloadErrorKey(null);
  }, [activeWorkspaceId, reloadToken]);

  const tree = useMemo(() => buildFileTree(files), [files]);
  const selectedCount = useMemo(
    () => countSelectedFiles(selectedPaths, files),
    [selectedPaths, files],
  );

  const handleExport = useCallback(async () => {
    if (!activeWorkspaceId || !revision || selectedCount === 0) return;
    const requestWorkspaceId = activeWorkspaceId;
    const requestGeneration = workspaceGeneration.current;
    setDownloadErrorKey(null);
    try {
      const blob = await officeApi.exportSelectedConfigZip(requestWorkspaceId, {
        revision,
        paths: [...selectedPaths],
      });
      if (
        workspaceGeneration.current !== requestGeneration ||
        lastWorkspaceId.current !== requestWorkspaceId
      ) {
        return;
      }
      triggerBlobDownload(blob, "kandev-config-selected.zip");
    } catch (error) {
      setDownloadErrorKey(
        error instanceof ApiError && error.status === 409
          ? "office:exportConfigurationChanged"
          : "office:failedToDownloadExport",
      );
    }
  }, [activeWorkspaceId, revision, selectedCount, selectedPaths]);

  const handleRetry = useCallback(() => {
    setReloadToken((value) => value + 1);
  }, []);

  const previewFile = files.find((f) => f.path === previewPath) ?? null;

  if (!activeWorkspaceId || loading || errorKey) {
    return (
      <ExportStatus
        activeWorkspaceId={activeWorkspaceId}
        loading={loading}
        errorKey={errorKey}
        onRetry={handleRetry}
      />
    );
  }

  return (
    <div className="flex flex-col h-full">
      <ExportToolbar
        workspaceName={workspaceName}
        selectedCount={selectedCount}
        total={files.length}
        onExport={() => void handleExport()}
      />
      {downloadErrorKey && (
        <ExportDownloadError errorKey={downloadErrorKey} onRetry={handleRetry} />
      )}
      <div className="flex min-h-0 flex-1">
        <ExportContent
          usesTouchDrawer={usesTouchDrawer}
          previewFile={previewFile}
          tree={tree}
          selectedPaths={selectedPaths}
          onSelectedPathsChange={setSelectedPaths}
          previewPath={previewPath}
          onPreviewPathChange={setPreviewPath}
        />
      </div>
    </div>
  );
}
