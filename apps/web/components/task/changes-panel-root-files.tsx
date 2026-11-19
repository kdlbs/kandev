"use client";

import { useState } from "react";
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
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { WorkspaceRootFilesResponse } from "@/lib/types/workspace-files";
import { ChangesPanelMoveRootFileDialog } from "./changes-panel-move-root-file-dialog";

type ChangesPanelRootFilesProps = {
  listing: WorkspaceRootFilesResponse | undefined;
  sessionId: string | null;
  onEditFile: (path: string, repo?: string) => void;
};

function WorkspaceRootFileRow({
  path,
  onEditFile,
  onMoveFile,
  touchMode,
}: {
  path: string;
  onEditFile: ChangesPanelRootFilesProps["onEditFile"];
  onMoveFile?: (path: string) => void;
  touchMode: boolean;
}) {
  const { t } = useTranslation();
  const openFile = () => onEditFile(path, undefined);
  return (
    <li
      className="group flex min-w-0 items-center gap-1 rounded-md px-1 text-sm hover:bg-muted/60"
      data-testid={`workspace-root-file-row-${path.replaceAll("/", "-")}`}
    >
      <button
        type="button"
        aria-label={path}
        className={`flex min-w-0 flex-1 items-center gap-2 text-left ${touchMode ? "min-h-11" : "min-h-8"}`}
        onClick={openFile}
      >
        <FileIcon fileName={path.split("/").at(-1) ?? path} className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate text-xs font-medium">{path}</span>
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            type="button"
            aria-label={t("common:showMoreActions")}
            className={`flex shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground ${touchMode ? "size-11" : "size-8 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"}`}
            onClick={(event) => event.stopPropagation()}
          >
            <IconDots className="size-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56 max-w-[calc(100vw-1rem)]">
          <DropdownMenuItem
            className={touchMode ? "min-h-11 cursor-pointer" : "cursor-pointer"}
            onSelect={openFile}
          >
            {t("task:openWorkspaceRootFile")}
          </DropdownMenuItem>
          {onMoveFile && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                className={touchMode ? "min-h-11 cursor-pointer" : "cursor-pointer"}
                onSelect={() => onMoveFile(path)}
              >
                {t("task:moveToRepository")}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </li>
  );
}

export function ChangesPanelRootFiles({
  listing,
  sessionId,
  onEditFile,
}: ChangesPanelRootFilesProps) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const [fileToMove, setFileToMove] = useState<string | null>(null);
  if (!listing?.supported || listing.files.length === 0) return null;

  const touchMode = isMobile || !isFinePointer;
  return (
    <section
      aria-labelledby="changes-workspace-root-files-heading"
      className="px-3 pb-3 pt-2"
      data-testid="changes-workspace-root-files"
    >
      <div className="mb-1 flex min-h-7 items-center gap-2">
        <h3
          id="changes-workspace-root-files-heading"
          className="min-w-0 flex-1 text-xs font-semibold text-muted-foreground"
        >
          {t("task:outsideRepositories")}
        </h3>
        <span aria-hidden="true" className="text-xs text-muted-foreground">
          ({listing.files.length})
        </span>
      </div>
      <ul className="flex flex-col gap-0.5">
        {listing.files.map((file) => (
          <WorkspaceRootFileRow
            key={file.path}
            path={file.path}
            onEditFile={onEditFile}
            onMoveFile={sessionId && listing.destinations.length > 0 ? setFileToMove : undefined}
            touchMode={touchMode}
          />
        ))}
      </ul>
      {listing.truncated && (
        <p className="mt-2 text-xs text-muted-foreground">
          {t("task:workspaceRootFilesTruncated")}
        </p>
      )}
      {sessionId && fileToMove && (
        <ChangesPanelMoveRootFileDialog
          open
          onOpenChange={(open) => {
            if (!open) setFileToMove(null);
          }}
          sessionId={sessionId}
          path={fileToMove}
          destinations={listing.destinations}
        />
      )}
    </section>
  );
}
