"use client";

import { useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  IconArrowBackUp,
  IconDots,
  IconLoader2,
  IconMinus,
  IconPencil,
  IconPlus,
} from "@tabler/icons-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { FileIcon } from "@/components/ui/file-icon";
import { FileStatusIcon } from "@/components/shared/file-status-icon";
import { SymlinkIndicator } from "@/components/shared/symlink-indicator";
import { LineStat } from "@/components/diff-stat";
import type { FileRowProps } from "./changes-panel-file-row";

export function TouchFileRowContent(props: FileRowProps) {
  const { file, treeMode, indentPx, isPending } = props;
  const slash = file.path.lastIndexOf("/");
  const name = file.path.slice(slash + 1);
  const folder = slash < 0 ? "" : file.path.slice(0, slash);

  return (
    <>
      <button
        type="button"
        title={file.path}
        className="flex min-h-11 min-w-0 flex-1 items-center gap-2 text-left cursor-pointer"
        style={indentPx ? { paddingLeft: indentPx } : undefined}
      >
        {isPending ? (
          <IconLoader2 className="size-3.5 shrink-0 animate-spin text-muted-foreground" />
        ) : (
          <FileIcon fileName={name} className="size-3.5 shrink-0" />
        )}
        <span className="min-w-0 flex-1">
          <span className="block whitespace-normal [overflow-wrap:anywhere] text-xs font-medium leading-4">
            {name}
          </span>
          <span className="flex min-w-0 items-center gap-2 text-[11px] leading-4 text-muted-foreground">
            {!treeMode && folder && <span className="min-w-0 truncate">{folder}</span>}
            <LineStat
              added={file.plus}
              removed={file.minus}
              className="shrink-0 text-[10px] gap-1"
            />
            <FileStatusIcon status={file.status} oldPath={file.oldPath} />
            <SymlinkIndicator isSymlink={file.isSymlink} />
          </span>
        </span>
      </button>
      <TouchFileRowActions {...props} />
    </>
  );
}

function TouchFileRowActions({
  file,
  isPending,
  onStage,
  onUnstage,
  onEditFile,
  onDiscard,
}: FileRowProps) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const StageIcon = file.staged ? IconMinus : IconPlus;
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          ref={triggerRef}
          type="button"
          aria-label={t("common:showMoreActions")}
          className="flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer"
          onClick={(event) => event.stopPropagation()}
        >
          <IconDots className="size-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="w-72 max-w-[calc(100vw-1rem)]"
        onClick={(event) => event.stopPropagation()}
      >
        <DropdownMenuLabel className="whitespace-normal [overflow-wrap:anywhere] text-xs font-normal text-muted-foreground">
          {file.path}
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          className="min-h-11 cursor-pointer"
          disabled={isPending}
          onSelect={() => (file.staged ? onUnstage : onStage)(file.path, file.repositoryName)}
        >
          {isPending ? <IconLoader2 className="animate-spin" /> : <StageIcon />}
          {t(file.staged ? "task:unstageFile" : "task:stageFile")}
        </DropdownMenuItem>
        <DropdownMenuItem
          className="min-h-11 cursor-pointer"
          onSelect={() => onEditFile(file.path, file.repositoryName)}
        >
          <IconPencil />
          {t("common:edit")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          variant="destructive"
          className="min-h-11 cursor-pointer"
          onSelect={() =>
            onDiscard(file.path, file.repositoryName, triggerRef.current ?? undefined)
          }
        >
          <IconArrowBackUp />
          {t("task:discardChanges2")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
