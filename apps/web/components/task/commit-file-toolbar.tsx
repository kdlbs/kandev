"use client";

import { useCallback, useState } from "react";
import {
  IconCopy,
  IconDots,
  IconFoldDown,
  IconTextWrap,
  IconLayoutColumns,
  IconLayoutRows,
  IconPencil,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { ExternalVcsFileMenuItem } from "@/components/editors/external-vcs-file-link";
import { DiffHeaderToolbarButtons, checkIsMarkdown } from "@/components/diff/diff-header-toolbar";
import { useGlobalViewMode } from "@/hooks/use-global-view-mode";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useTranslation } from "react-i18next";

const mobileMenuItem = "cursor-pointer gap-3 text-sm";
const mobileMenuIcon = "size-4 text-muted-foreground";

export type CommitFileToolbarProps = {
  filePath: string;
  diff: string;
  wordWrap: boolean;
  onToggleWordWrap: () => void;
  expandUnchanged?: boolean;
  onToggleExpandUnchanged?: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  repo?: string;
  sessionId?: string | null;
  taskId?: string | null;
  repositoryId?: string | null;
  status?: string | null;
  previousPath?: string | null;
  publishedBranch?: string | null;
  baseBranch?: string | null;
};

function CommitFileToolbarMenuItems({
  filePath,
  diff,
  wordWrap,
  onToggleWordWrap,
  expandUnchanged,
  onToggleExpandUnchanged,
  onOpenFile,
  repo,
  sessionId,
  taskId,
  repositoryId,
  status,
  previousPath,
  publishedBranch,
  baseBranch,
  viewMode,
  onToggleViewMode,
}: CommitFileToolbarProps & {
  viewMode: "split" | "unified";
  onToggleViewMode: () => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenuLabel className="truncate font-medium text-foreground" title={filePath}>
        {filePath.split("/").pop() || filePath}
      </DropdownMenuLabel>
      <DropdownMenuItem
        className={mobileMenuItem}
        onSelect={() => void copyToClipboard(diff || "")}
      >
        <IconCopy className={mobileMenuIcon} />
        {t("diff:copyDiff")}
      </DropdownMenuItem>
      {onOpenFile && (
        <DropdownMenuItem className={mobileMenuItem} onSelect={() => onOpenFile(filePath, repo)}>
          <IconPencil className={mobileMenuIcon} />
          {t("common:edit")}
        </DropdownMenuItem>
      )}
      <ExternalVcsFileMenuItem
        filePath={filePath}
        previousPath={previousPath}
        status={status}
        taskId={taskId}
        sessionId={sessionId}
        repositoryId={repositoryId}
        repositoryName={repo}
        publishedBranch={publishedBranch}
        baseBranch={baseBranch}
      />
      <DropdownMenuSeparator />
      {onToggleExpandUnchanged && (
        <DropdownMenuCheckboxItem
          checked={expandUnchanged === true}
          className={mobileMenuItem}
          onCheckedChange={onToggleExpandUnchanged}
        >
          <IconFoldDown className={mobileMenuIcon} />
          {t("diff:expandAllLines")}
        </DropdownMenuCheckboxItem>
      )}
      <DropdownMenuCheckboxItem
        checked={wordWrap}
        className={mobileMenuItem}
        onCheckedChange={onToggleWordWrap}
      >
        <IconTextWrap className={mobileMenuIcon} />
        {t("diff:toggleWordWrap")}
      </DropdownMenuCheckboxItem>
      <DropdownMenuItem className={mobileMenuItem} onSelect={onToggleViewMode}>
        {viewMode === "split" ? (
          <IconLayoutRows className={mobileMenuIcon} />
        ) : (
          <IconLayoutColumns className={mobileMenuIcon} />
        )}
        {viewMode === "split" ? t("diff:switchToUnifiedView") : t("diff:switchToSplitView")}
      </DropdownMenuItem>
    </>
  );
}

function CommitFileToolbarMenu(props: CommitFileToolbarProps) {
  const { t } = useTranslation();
  const [viewMode, setViewMode] = useGlobalViewMode();
  const [open, setOpen] = useState(false);
  const toggleViewMode = useCallback(
    () => setViewMode(viewMode === "split" ? "unified" : "split"),
    [setViewMode, viewMode],
  );

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={t("review:moreActionsFor", { filePath: props.filePath })}
          title={t("review:moreActionsFor", { filePath: props.filePath })}
          className="size-11 shrink-0 cursor-pointer text-muted-foreground transition-[scale,color,background-color] duration-150 ease-out active:scale-[0.96]"
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => setOpen((previous) => !previous)}
        >
          <IconDots className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        data-testid="commit-file-actions-menu"
        aria-label={t("review:actionsFor", { filePath: props.filePath })}
        align="end"
        className="w-64"
      >
        <CommitFileToolbarMenuItems
          {...props}
          viewMode={viewMode}
          onToggleViewMode={toggleViewMode}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function CommitFileToolbar(props: CommitFileToolbarProps) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const [viewMode, setViewMode] = useGlobalViewMode();
  const toggleViewMode = useCallback(
    () => setViewMode(viewMode === "split" ? "unified" : "split"),
    [setViewMode, viewMode],
  );

  if (isMobile || isFinePointer === false) return <CommitFileToolbarMenu {...props} />;

  return (
    <div className="flex items-center gap-0.5">
      <DiffHeaderToolbarButtons
        resolvedPath={props.filePath}
        onCopyDiff={() => void copyToClipboard(props.diff || "")}
        isMarkdownFile={checkIsMarkdown(props.filePath)}
        wordWrap={props.wordWrap}
        onToggleWordWrap={props.onToggleWordWrap}
        viewMode={viewMode}
        onToggleViewMode={toggleViewMode}
        onOpenFile={props.onOpenFile}
        repo={props.repo}
        expandUnchanged={props.expandUnchanged}
        onToggleExpandUnchanged={props.onToggleExpandUnchanged}
        externalLink={{
          taskId: props.taskId,
          sessionId: props.sessionId,
          repositoryId: props.repositoryId,
          repositoryName: props.repo,
          status: props.status,
          previousPath: props.previousPath,
          publishedBranch: props.publishedBranch,
          baseBranch: props.baseBranch,
        }}
        externalLinkSize="xs"
      />
      <span className="sr-only">{t("task:commitChanges")}</span>
    </div>
  );
}
