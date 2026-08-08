"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import * as officeApi from "@/lib/api/domains/office-api";
import { ExportFileTree } from "./export-file-tree";
import { ExportFilePreview } from "./export-file-preview";
import { buildFileTree, bundleToExportFiles, countSelectedFiles } from "./export-utils";
import type { ExportFile } from "./export-types";
import { useTranslation } from "react-i18next";

export function ExportPreview() {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const workspaces = useAppStore((s) => s.workspaces);
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  // Fallback LABEL for the bundle header, shown only before the workspace loads.
  const workspaceName = activeWorkspace?.name || t("common:workspace");

  const [files, setFiles] = useState<ExportFile[]>([]);
  const [loading, setLoading] = useState(true);
  // A catalog KEY, not a message: the load effect below must not take `t` in
  // its dep array, or a locale switch would re-issue the export request.
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [previewPath, setPreviewPath] = useState<string | null>(null);

  useEffect(() => {
    if (!activeWorkspaceId) return;
    let cancelled = false;
    officeApi
      .exportConfig(activeWorkspaceId)
      .then((res) => {
        if (cancelled) return;
        const exported = bundleToExportFiles(res.bundle);
        setFiles(exported);
        setSelectedPaths(new Set(exported.map((f) => f.path)));
        if (exported.length > 0) setPreviewPath(exported[0].path);
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
  }, [activeWorkspaceId]);

  const tree = useMemo(() => buildFileTree(files), [files]);
  const selectedCount = useMemo(
    () => countSelectedFiles(selectedPaths, files),
    [selectedPaths, files],
  );

  const handleExport = useCallback(() => {
    if (!activeWorkspaceId) return;
    // This same-origin endpoint is a file download, not an external navigation.
    window.open(officeApi.exportConfigZipUrl(activeWorkspaceId), "_blank");
  }, [activeWorkspaceId]);

  const previewFile = files.find((f) => f.path === previewPath) ?? null;

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        {t("office:loadingExportBundle")}
      </div>
    );
  }

  if (errorKey) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-destructive">
        {t(errorKey)}
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
        <span className="text-sm text-muted-foreground">
          {t("office:exportSelectionSummary", {
            workspace: workspaceName,
            selected: selectedCount,
            total: files.length,
          })}
        </span>
        <Button
          size="sm"
          onClick={handleExport}
          disabled={selectedCount === 0}
          className="cursor-pointer"
        >
          <IconDownload className="h-4 w-4 mr-1.5" />
          {t("office:exportFilesCount", { count: selectedCount })}
        </Button>
      </div>
      <div className="flex flex-1 min-h-0">
        <ExportFileTree
          tree={tree}
          selectedPaths={selectedPaths}
          onSelectedPathsChange={setSelectedPaths}
          previewPath={previewPath}
          onPreviewPathChange={setPreviewPath}
        />
        <ExportFilePreview file={previewFile} />
      </div>
    </div>
  );
}
