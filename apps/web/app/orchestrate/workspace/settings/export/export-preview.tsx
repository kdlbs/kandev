"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconDownload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";
import { ExportFileTree } from "./export-file-tree";
import { ExportFilePreview } from "./export-file-preview";
import { buildFileTree, bundleToExportFiles, countSelectedFiles } from "./export-utils";
import type { ExportFile } from "./export-types";

export function ExportPreview() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const workspaces = useAppStore((s) => s.workspaces);
  const activeWorkspace = workspaces.items.find((w) => w.id === workspaces.activeId);
  const workspaceName = activeWorkspace?.name || "Workspace";

  const [files, setFiles] = useState<ExportFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [previewPath, setPreviewPath] = useState<string | null>(null);

  useEffect(() => {
    if (!activeWorkspaceId) return;
    let cancelled = false;
    orchestrateApi
      .exportConfig(activeWorkspaceId)
      .then((res) => {
        if (cancelled) return;
        const exported = bundleToExportFiles(res.bundle);
        setFiles(exported);
        setSelectedPaths(new Set(exported.map((f) => f.path)));
        if (exported.length > 0) setPreviewPath(exported[0].path);
      })
      .catch(() => {
        if (!cancelled) setError("Failed to load export bundle");
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
    window.open(orchestrateApi.exportConfigZipUrl(activeWorkspaceId), "_blank");
  }, [activeWorkspaceId]);

  const previewFile = files.find((f) => f.path === previewPath) ?? null;

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-muted-foreground">
        Loading export bundle...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-destructive">
        {error}
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
        <span className="text-sm text-muted-foreground">
          {workspaceName} export &middot; {selectedCount} / {files.length} files selected
        </span>
        <Button
          size="sm"
          onClick={handleExport}
          disabled={selectedCount === 0}
          className="cursor-pointer"
        >
          <IconDownload className="h-4 w-4 mr-1.5" />
          Export {selectedCount} files
        </Button>
      </div>
      <div className="flex flex-1 min-h-0">
        <ExportFileTree
          tree={tree}
          files={files}
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
