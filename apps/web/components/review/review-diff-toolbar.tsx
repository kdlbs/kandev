"use client";

import { useCallback, useState } from "react";
import {
  IconArrowBackUp,
  IconCopy,
  IconDots,
  IconEye,
  IconFold,
  IconFoldDown,
  IconLayoutColumns,
  IconLayoutRows,
  IconPencil,
  IconTextWrap,
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  FileActionsDropdown,
  FileActionsMenuItems,
} from "@/components/editors/file-actions-dropdown";
import {
  ExternalVcsFileLink,
  ExternalVcsFileMenuItem,
} from "@/components/editors/external-vcs-file-link";
import { useGlobalViewMode } from "@/hooks/use-global-view-mode";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { isMarkdownFile } from "@/lib/utils/file-types";

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";
const iconBtnActive = "h-6 w-6 p-0 cursor-pointer bg-muted opacity-100";
const iconBtnDestructive =
  "h-6 w-6 p-0 cursor-pointer opacity-60 hover:text-destructive hover:opacity-100";
const mobileMenuItem = "cursor-pointer gap-3 text-sm";
const mobileMenuIcon = "size-4 text-muted-foreground";

export type FileDiffToolbarProps = {
  diff: string;
  filePath: string;
  previousPath?: string | null;
  status?: string | null;
  taskId?: string | null;
  sessionId: string;
  repositoryId?: string | null;
  source: string;
  publishedBranch?: string | null;
  baseBranch?: string | null;
  wordWrap: boolean;
  expandUnchanged: boolean;
  onDiscard: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  onToggleMarkdownPreview?: () => void;
  markdownPreview?: boolean;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
  /** Multi-repo subpath (repository_name) so the Edit action opens the file
   *  under the right repository instead of the bare task root. */
  repo?: string;
};

function ToolbarIconBtn({
  onClick,
  tooltip,
  active,
  children,
  className,
}: {
  onClick: () => void;
  tooltip: string;
  active?: boolean;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          aria-label={tooltip}
          aria-pressed={active}
          variant="ghost"
          size="sm"
          className={className ?? (active ? iconBtnActive : iconBtn)}
          onClick={onClick}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

type DiffDisplayControlsProps = {
  expandUnchanged: boolean;
  globalViewMode: "split" | "unified";
  wordWrap: boolean;
  onToggleExpandUnchanged: () => void;
  onToggleViewMode: () => void;
  onToggleWordWrap: () => void;
};

function DiffDisplayControls({
  expandUnchanged,
  globalViewMode,
  wordWrap,
  onToggleExpandUnchanged,
  onToggleViewMode,
  onToggleWordWrap,
}: DiffDisplayControlsProps) {
  return (
    <>
      <ToolbarIconBtn
        onClick={onToggleExpandUnchanged}
        tooltip={expandUnchanged ? "Collapse unchanged" : "Expand all"}
        active={expandUnchanged}
      >
        {expandUnchanged ? (
          <IconFold className="h-3.5 w-3.5" />
        ) : (
          <IconFoldDown className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
      <ToolbarIconBtn onClick={onToggleWordWrap} tooltip="Toggle word wrap" active={wordWrap}>
        <IconTextWrap className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={onToggleViewMode}
        tooltip={globalViewMode === "split" ? "Switch to unified view" : "Switch to split view"}
      >
        {globalViewMode === "split" ? (
          <IconLayoutRows className="h-3.5 w-3.5" />
        ) : (
          <IconLayoutColumns className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
    </>
  );
}

function MobileDiffViewMenuItems({
  expandUnchanged,
  wordWrap,
  onToggleExpandUnchanged,
  onToggleWordWrap,
}: Pick<
  FileDiffToolbarProps,
  "expandUnchanged" | "wordWrap" | "onToggleExpandUnchanged" | "onToggleWordWrap"
>) {
  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuLabel>View</DropdownMenuLabel>
      <DropdownMenuCheckboxItem
        checked={expandUnchanged}
        className={mobileMenuItem}
        onCheckedChange={onToggleExpandUnchanged}
      >
        <IconFoldDown className={mobileMenuIcon} />
        Expand unchanged lines
      </DropdownMenuCheckboxItem>
      <DropdownMenuCheckboxItem
        checked={wordWrap}
        className={mobileMenuItem}
        onCheckedChange={onToggleWordWrap}
      >
        <IconTextWrap className={mobileMenuIcon} />
        Wrap long lines
      </DropdownMenuCheckboxItem>
    </>
  );
}

function MobileFileActionsMenu(props: FileDiffToolbarProps) {
  const {
    diff,
    filePath,
    previousPath,
    status,
    taskId,
    sessionId,
    repositoryId,
    source,
    publishedBranch,
    baseBranch,
    wordWrap,
    expandUnchanged,
    onDiscard,
    onOpenFile,
    onToggleMarkdownPreview,
    markdownPreview,
    onToggleExpandUnchanged,
    onToggleWordWrap,
    repo,
  } = props;
  const handleCopyDiff = useCallback(() => {
    void copyToClipboard(diff || "");
  }, [diff]);
  const [open, setOpen] = useState(false);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={`More actions for ${filePath}`}
          title={`More actions for ${filePath}`}
          className="size-11 shrink-0 cursor-pointer text-muted-foreground transition-[scale,color,background-color] duration-150 ease-out active:scale-[0.96]"
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => setOpen((previous) => !previous)}
        >
          <IconDots className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        data-testid="review-file-actions-menu"
        aria-label={`Actions for ${filePath}`}
        align="end"
        className="w-64"
      >
        <DropdownMenuLabel className="truncate font-medium text-foreground" title={filePath}>
          {filePath.split("/").pop() || filePath}
        </DropdownMenuLabel>
        <DropdownMenuItem className={mobileMenuItem} onSelect={handleCopyDiff}>
          <IconCopy className={mobileMenuIcon} />
          Copy diff
        </DropdownMenuItem>
        {onOpenFile && (
          <DropdownMenuItem className={mobileMenuItem} onSelect={() => onOpenFile(filePath, repo)}>
            <IconPencil className={mobileMenuIcon} />
            Edit file
          </DropdownMenuItem>
        )}
        {onToggleMarkdownPreview && isMarkdownFile(filePath) && (
          <DropdownMenuItem className={mobileMenuItem} onSelect={onToggleMarkdownPreview}>
            <IconEye className={mobileMenuIcon} />
            {markdownPreview ? "Show diff" : "Preview markdown"}
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
        <FileActionsMenuItems filePath={filePath} sessionId={sessionId} />
        <MobileDiffViewMenuItems
          expandUnchanged={expandUnchanged}
          wordWrap={wordWrap}
          onToggleExpandUnchanged={onToggleExpandUnchanged}
          onToggleWordWrap={onToggleWordWrap}
        />
        {source === "uncommitted" && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" className={mobileMenuItem} onSelect={onDiscard}>
              <IconArrowBackUp className="size-4" />
              Revert changes
            </DropdownMenuItem>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function DesktopFileDiffToolbar(props: FileDiffToolbarProps) {
  const {
    diff,
    filePath,
    previousPath,
    status,
    taskId,
    sessionId,
    repositoryId,
    source,
    publishedBranch,
    baseBranch,
    wordWrap,
    expandUnchanged,
    onDiscard,
    onOpenFile,
    onToggleMarkdownPreview,
    markdownPreview,
    onToggleExpandUnchanged,
    onToggleWordWrap,
    repo,
  } = props;
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const handleCopyDiff = useCallback(() => {
    void copyToClipboard(diff || "");
  }, [diff]);
  const handleToggleViewMode = useCallback(
    () => setGlobalViewMode(globalViewMode === "split" ? "unified" : "split"),
    [globalViewMode, setGlobalViewMode],
  );

  return (
    <div className="flex items-center gap-0.5">
      <ToolbarIconBtn onClick={handleCopyDiff} tooltip="Copy diff">
        <IconCopy className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ExternalVcsFileLink
        filePath={filePath}
        previousPath={previousPath}
        status={status}
        taskId={taskId}
        sessionId={sessionId}
        repositoryId={repositoryId}
        repositoryName={repo}
        publishedBranch={publishedBranch}
        baseBranch={baseBranch}
        size="xs"
      />
      <DiffDisplayControls
        expandUnchanged={expandUnchanged}
        globalViewMode={globalViewMode}
        wordWrap={wordWrap}
        onToggleExpandUnchanged={onToggleExpandUnchanged}
        onToggleViewMode={handleToggleViewMode}
        onToggleWordWrap={onToggleWordWrap}
      />
      {onToggleMarkdownPreview && isMarkdownFile(filePath) && (
        <ToolbarIconBtn
          onClick={onToggleMarkdownPreview}
          tooltip={markdownPreview ? "Show diff" : "Preview markdown"}
        >
          <IconEye className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      {onOpenFile && (
        <ToolbarIconBtn onClick={() => onOpenFile(filePath, repo)} tooltip="Edit">
          <IconPencil className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      <FileActionsDropdown filePath={filePath} sessionId={sessionId} size="xs" />
      {source === "uncommitted" && (
        <ToolbarIconBtn onClick={onDiscard} tooltip="Revert changes" className={iconBtnDestructive}>
          <IconArrowBackUp className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
    </div>
  );
}

export function FileDiffToolbar(props: FileDiffToolbarProps) {
  const { isMobile } = useResponsiveBreakpoint();
  return isMobile ? <MobileFileActionsMenu {...props} /> : <DesktopFileDiffToolbar {...props} />;
}
