"use client";

import { useEffect, useMemo, useState } from "react";
import { IconDots } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { FileIcon } from "@/components/ui/file-icon";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { VisibleRow } from "@/hooks/use-tree";
import type { WorkspaceRootFilesResponse } from "@/lib/types/workspace-files";
import { cn } from "@/lib/utils";
import { ChangesPanelFileFolder, ChangesPanelFileName } from "./changes-panel-file-name";
import { ChangesPanelGroupHeader } from "./changes-panel-group-header";
import { buildChangesTree, flattenChangesTree } from "./changes-file-tree-model";
import type { ChangesTreeNode } from "./changes-file-tree-model";
import { TreeDirRow } from "./changes-panel-tree";
import { ChangesPanelMoveRootFileDialog } from "./changes-panel-move-root-file-dialog";

type ChangesPanelRootFilesProps = {
  contextKey: string;
  listing: WorkspaceRootFilesResponse | undefined;
  sessionId: string | null;
  onEditFile: (path: string, repo?: string) => void;
};

type RootFile = WorkspaceRootFilesResponse["files"][number];

export function ChangesPanelRootFiles(props: ChangesPanelRootFilesProps) {
  return <ChangesPanelRootFilesContent key={props.contextKey} {...props} />;
}

function ChangesPanelRootFilesContent({
  listing,
  sessionId,
  onEditFile,
}: ChangesPanelRootFilesProps) {
  const layout = useAppStore((state) => state.userSettings.changesPanelLayout);
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const [collapsed, setCollapsed] = useState(false);
  const [collapsedDirectories, setCollapsedDirectories] = useState<Set<string>>(() => new Set());
  const [fileToMove, setFileToMove] = useState<{ path: string; sessionId: string } | null>(null);
  const treeMode = layout === "tree";
  const touchMode = isMobile || !isFinePointer;
  const visibleTreeRows = useMemo(() => {
    if (!treeMode || !listing?.files.length) return [];
    return flattenChangesTree(buildChangesTree(listing.files), (path) =>
      collapsedDirectories.has(path),
    );
  }, [collapsedDirectories, listing?.files, treeMode]);

  useEffect(() => setFileToMove(null), [sessionId]);
  if (!listing?.supported || listing.files.length === 0) return null;

  const toggleDirectory = (path: string) => {
    setCollapsedDirectories((current) => toggleSetValue(current, path));
  };
  const openMoveDialog = (path: string) => {
    if (sessionId) setFileToMove({ path, sessionId });
  };

  return (
    <>
      <ChangesPanelRootFilesSection
        listing={listing}
        collapsed={collapsed}
        treeMode={treeMode}
        touchMode={touchMode}
        visibleTreeRows={visibleTreeRows}
        onToggleGroup={() => setCollapsed((current) => !current)}
        onToggleDirectory={toggleDirectory}
        onEditFile={onEditFile}
        onMoveFile={sessionId && listing.destinations.length > 0 ? openMoveDialog : undefined}
      />
      {sessionId && fileToMove?.sessionId === sessionId && (
        <ChangesPanelMoveRootFileDialog
          key={`${fileToMove.sessionId}:${fileToMove.path}`}
          open
          onOpenChange={(open) => {
            if (!open) {
              setFileToMove((selection) =>
                selection?.sessionId === fileToMove.sessionId ? null : selection,
              );
            }
          }}
          sessionId={fileToMove.sessionId}
          path={fileToMove.path}
          destinations={listing.destinations}
        />
      )}
    </>
  );
}

function ChangesPanelRootFilesSection({
  listing,
  collapsed,
  treeMode,
  touchMode,
  visibleTreeRows,
  onToggleGroup,
  onToggleDirectory,
  onEditFile,
  onMoveFile,
}: {
  listing: WorkspaceRootFilesResponse;
  collapsed: boolean;
  treeMode: boolean;
  touchMode: boolean;
  visibleTreeRows: VisibleRow<ChangesTreeNode<RootFile>>[];
  onToggleGroup: () => void;
  onToggleDirectory: (path: string) => void;
  onEditFile: ChangesPanelRootFilesProps["onEditFile"];
  onMoveFile?: (path: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <section
      aria-labelledby="changes-workspace-root-files-heading"
      className="pb-3 pt-2"
      data-testid="changes-workspace-root-files"
    >
      <div className={treeMode && !touchMode ? "-ml-2" : undefined}>
        <div className="mb-1 flex items-center justify-between gap-2 px-1 py-0.5">
          <ChangesPanelGroupHeader
            id="changes-workspace-root-files-heading"
            label={t("task:outsideRepositories")}
            count={listing.files.length}
            collapsed={collapsed}
            onToggle={onToggleGroup}
            testId="changes-root-files-toggle"
            touchMode={touchMode}
          />
        </div>
      </div>
      {!collapsed && (
        <>
          {treeMode ? (
            <ul className="flex flex-col gap-0.5">
              {visibleTreeRows.map((row) =>
                row.isDir ? (
                  <li key={`directory:${row.path}`}>
                    <TreeDirRow
                      row={row}
                      baseIndentPx={12}
                      touchMode={touchMode}
                      onToggle={() => onToggleDirectory(row.path)}
                    />
                  </li>
                ) : (
                  <WorkspaceRootFileRow
                    key={`file:${row.path}`}
                    path={row.path}
                    treeMode
                    touchMode={touchMode}
                    indentPx={12 + row.depth * 12}
                    onEditFile={onEditFile}
                    onMoveFile={onMoveFile}
                  />
                ),
              )}
            </ul>
          ) : (
            <ul className="flex flex-col gap-0.5">
              {listing.files.map((file) => (
                <WorkspaceRootFileRow
                  key={file.path}
                  path={file.path}
                  treeMode={false}
                  touchMode={touchMode}
                  indentPx={0}
                  onEditFile={onEditFile}
                  onMoveFile={onMoveFile}
                />
              ))}
            </ul>
          )}
          {listing.truncated && (
            <p className="mt-2 px-3 text-xs text-muted-foreground">
              {t("task:workspaceRootFilesTruncated")}
            </p>
          )}
        </>
      )}
    </section>
  );
}

function WorkspaceRootFileRow({
  path,
  treeMode,
  touchMode,
  indentPx,
  onEditFile,
  onMoveFile,
}: {
  path: string;
  treeMode: boolean;
  touchMode: boolean;
  indentPx: number;
  onEditFile: ChangesPanelRootFilesProps["onEditFile"];
  onMoveFile?: (path: string) => void;
}) {
  const { folder, name } = splitRootFilePath(path);
  const openFile = () => onEditFile(path, undefined);

  return (
    <li
      className={cn(
        "group flex min-w-0 items-center gap-1 rounded-md px-1 text-sm hover:bg-muted/60",
        !touchMode && "-mx-1",
      )}
      data-testid={`workspace-root-file-row-${path.replaceAll("/", "-")}`}
    >
      <RootFileOpenButton
        path={path}
        name={name}
        folder={folder}
        treeMode={treeMode}
        touchMode={touchMode}
        indentPx={indentPx}
        onEditFile={onEditFile}
      />
      <RootFileActions
        path={path}
        touchMode={touchMode}
        onOpenFile={openFile}
        onMoveFile={onMoveFile}
      />
    </li>
  );
}

function RootFileOpenButton({
  path,
  name,
  folder,
  treeMode,
  touchMode,
  indentPx,
  onEditFile,
}: {
  path: string;
  name: string;
  folder: string;
  treeMode: boolean;
  touchMode: boolean;
  indentPx: number;
  onEditFile: ChangesPanelRootFilesProps["onEditFile"];
}) {
  const rowIndentPx = Math.min(indentPx, 24);
  return (
    <button
      type="button"
      aria-label={path}
      title={path}
      className={cn(
        "flex min-w-0 flex-1 items-center gap-2 text-left cursor-pointer",
        touchMode ? "min-h-11" : "min-h-8",
      )}
      style={
        rowIndentPx || !treeMode ? { paddingLeft: rowIndentPx + (treeMode ? 0 : 4) } : undefined
      }
      onClick={() => onEditFile(path, undefined)}
    >
      <FileIcon fileName={name} className={cn("shrink-0", touchMode ? "size-3.5" : "size-4")} />
      <RootFileName folder={folder} name={name} treeMode={treeMode} touchMode={touchMode} />
    </button>
  );
}

function RootFileName({
  folder,
  name,
  treeMode,
  touchMode,
}: {
  folder: string;
  name: string;
  treeMode: boolean;
  touchMode: boolean;
}) {
  if (touchMode) {
    return (
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <ChangesPanelFileName name={name} touchMode />
        {!treeMode && folder && (
          <span className="flex min-w-0 text-[11px] leading-4 text-muted-foreground">
            <ChangesPanelFileFolder folder={folder} touchMode />
          </span>
        )}
      </span>
    );
  }

  return (
    <p className="flex min-w-0 text-foreground text-xs">
      {!treeMode && folder && <ChangesPanelFileFolder folder={folder} trailingSlash />}
      <ChangesPanelFileName name={name} />
    </p>
  );
}

function RootFileActions({
  path,
  touchMode,
  onOpenFile,
  onMoveFile,
}: {
  path: string;
  touchMode: boolean;
  onOpenFile: () => void;
  onMoveFile?: (path: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={t("common:showMoreActions")}
          className={cn(
            "flex shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground",
            touchMode
              ? "size-11"
              : "size-8 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100",
          )}
          onClick={(event) => event.stopPropagation()}
        >
          <IconDots className="size-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56 max-w-[calc(100vw-1rem)]">
        <DropdownMenuItem
          className={cn("cursor-pointer", touchMode && "min-h-11")}
          onSelect={onOpenFile}
        >
          {t("task:openWorkspaceRootFile")}
        </DropdownMenuItem>
        {onMoveFile && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className={cn("cursor-pointer", touchMode && "min-h-11")}
              onSelect={() => onMoveFile(path)}
            >
              {t("task:moveToRepository")}
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function splitRootFilePath(path: string): { folder: string; name: string } {
  const slashIndex = path.lastIndexOf("/");
  return {
    folder: slashIndex === -1 ? "" : path.slice(0, slashIndex),
    name: slashIndex === -1 ? path : path.slice(slashIndex + 1),
  };
}

function toggleSetValue<T>(current: Set<T>, value: T): Set<T> {
  const next = new Set(current);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}
