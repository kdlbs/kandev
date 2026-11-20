"use client";

import { useRef, useState } from "react";
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
  IconMessagePlus,
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
import { useCopyRepositoryPath } from "@/hooks/use-copy-repository-path";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { getFilePreviewKind, isMarkdownFile } from "@/lib/utils/file-types";
import { useTranslation } from "react-i18next";
import { HtmlPreviewToolbarButton, useHtmlPreviewLabel } from "./review-html-preview-actions";

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";
const iconBtnActive = "h-6 w-6 p-0 cursor-pointer bg-muted opacity-100";
const iconBtnDestructive =
  "h-6 w-6 p-0 cursor-pointer opacity-60 hover:text-destructive hover:opacity-100";
const mobileMenuItem = "cursor-pointer gap-3 text-sm";
const mobileMenuItemTouch = `${mobileMenuItem} min-h-11`;
const mobileMenuIcon = "size-4 text-muted-foreground";

export type FileDiffToolbarProps = {
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
  onCommentFile?: () => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  onToggleMarkdownPreview?: () => void;
  markdownPreview?: boolean;
  /** Set only for eligible `.html`/`.htm` rows; the row owner excludes deleted and binary files. */
  onToggleHtmlPreview?: () => void;
  htmlPreview?: boolean;
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

function HtmlPreviewMenuItem({
  previewing,
  onToggle,
}: {
  previewing?: boolean;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const label = useHtmlPreviewLabel(previewing);
  return (
    <>
      <DropdownMenuItem
        className={mobileMenuItemTouch}
        data-testid="review-html-preview-menu-item"
        onSelect={onToggle}
      >
        <IconEye className={mobileMenuIcon} />
        {label}
      </DropdownMenuItem>
      <div
        data-testid="review-html-preview-help"
        className="px-2 pb-2 text-xs leading-snug text-muted-foreground"
      >
        <p>{t("review:htmlPreviewWorkspaceSource")}</p>
        <p className="mt-1">{t("task:htmlPreviewTrustedCode")}</p>
      </div>
    </>
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
  const { t } = useTranslation();
  return (
    <>
      <ToolbarIconBtn
        onClick={onToggleExpandUnchanged}
        tooltip={expandUnchanged ? t("review:collapseUnchanged") : t("review:expandAll")}
        active={expandUnchanged}
      >
        {expandUnchanged ? (
          <IconFold className="h-3.5 w-3.5" />
        ) : (
          <IconFoldDown className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={onToggleWordWrap}
        tooltip={t("review:toggleWordWrap")}
        active={wordWrap}
      >
        <IconTextWrap className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={onToggleViewMode}
        tooltip={
          globalViewMode === "split"
            ? t("review:switchToUnifiedView")
            : t("review:switchToSplitView")
        }
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
  const { t } = useTranslation();
  return (
    <>
      <DropdownMenuSeparator />
      <DropdownMenuLabel>{t("review:view")}</DropdownMenuLabel>
      <DropdownMenuCheckboxItem
        checked={expandUnchanged}
        className={mobileMenuItem}
        onCheckedChange={onToggleExpandUnchanged}
      >
        <IconFoldDown className={mobileMenuIcon} />
        {t("review:expandUnchangedLines")}
      </DropdownMenuCheckboxItem>
      <DropdownMenuCheckboxItem
        checked={wordWrap}
        className={mobileMenuItem}
        onCheckedChange={onToggleWordWrap}
      >
        <IconTextWrap className={mobileMenuIcon} />
        {t("review:wrapLongLines")}
      </DropdownMenuCheckboxItem>
    </>
  );
}

function MobileFileMenuItems(props: FileDiffToolbarProps) {
  const { t } = useTranslation();
  const copyRepositoryPath = useCopyRepositoryPath();
  const {
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
    onToggleHtmlPreview,
    htmlPreview,
    onToggleExpandUnchanged,
    onToggleWordWrap,
    repo,
  } = props;
  const showHtmlPreview = Boolean(onToggleHtmlPreview) && getFilePreviewKind(filePath) === "html";
  return (
    <>
      <DropdownMenuItem
        className={mobileMenuItemTouch}
        onSelect={() => void copyRepositoryPath(filePath)}
      >
        <IconCopy className={mobileMenuIcon} />
        {t("task:copyPath")}
      </DropdownMenuItem>
      {onOpenFile && (
        <DropdownMenuItem className={mobileMenuItem} onSelect={() => onOpenFile(filePath, repo)}>
          <IconPencil className={mobileMenuIcon} />
          {t("review:editFile")}
        </DropdownMenuItem>
      )}
      {onToggleMarkdownPreview && isMarkdownFile(filePath) && (
        <DropdownMenuItem className={mobileMenuItem} onSelect={onToggleMarkdownPreview}>
          <IconEye className={mobileMenuIcon} />
          {markdownPreview ? t("review:showDiff") : t("review:previewMarkdown")}
        </DropdownMenuItem>
      )}
      {showHtmlPreview && onToggleHtmlPreview && (
        <HtmlPreviewMenuItem previewing={htmlPreview} onToggle={onToggleHtmlPreview} />
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
      <FileActionsMenuItems
        filePath={filePath}
        sessionId={sessionId}
        repositoryId={repositoryId}
        repositoryName={repo}
        includeCopyPath={false}
      />
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
            {t("review:revertChanges")}
          </DropdownMenuItem>
        </>
      )}
    </>
  );
}

function MobileFileActionsMenu(props: FileDiffToolbarProps) {
  const { t } = useTranslation();
  const { filePath, onCommentFile } = props;
  const [open, setOpen] = useState(false);
  const commentSelectedRef = useRef(false);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          data-review-comment-opener
          aria-label={t("review:moreActionsFor", { filePath })}
          title={t("review:moreActionsFor", { filePath })}
          className="size-11 shrink-0 cursor-pointer text-muted-foreground transition-[scale,color,background-color] duration-150 ease-out active:scale-[0.96]"
          onPointerDown={(event) => event.preventDefault()}
          onClick={() => setOpen((previous) => !previous)}
        >
          <IconDots className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        onCloseAutoFocus={(event) => {
          if (!commentSelectedRef.current) return;
          event.preventDefault();
          commentSelectedRef.current = false;
          requestAnimationFrame(() => onCommentFile?.());
        }}
        data-testid="review-file-actions-menu"
        aria-label={t("review:actionsFor", { filePath })}
        align="end"
        className="w-64 mobile-review-file-actions-menu"
      >
        <DropdownMenuLabel className="truncate font-medium text-foreground" title={filePath}>
          {filePath.split("/").pop() || filePath}
        </DropdownMenuLabel>
        {onCommentFile && (
          <DropdownMenuItem
            className={mobileMenuItemTouch}
            onSelect={() => {
              commentSelectedRef.current = true;
            }}
          >
            <IconMessagePlus className={mobileMenuIcon} />
            {t("review:commentOnFile")}
          </DropdownMenuItem>
        )}
        <MobileFileMenuItems {...props} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

type DesktopFileActionsProps = Pick<
  FileDiffToolbarProps,
  | "filePath"
  | "sessionId"
  | "repositoryId"
  | "source"
  | "repo"
  | "onDiscard"
  | "onOpenFile"
  | "onToggleMarkdownPreview"
  | "markdownPreview"
  | "onToggleHtmlPreview"
  | "htmlPreview"
> & { showHtmlPreview: boolean };

function DesktopFileActions(props: DesktopFileActionsProps) {
  const { t } = useTranslation();
  const {
    filePath,
    sessionId,
    repositoryId,
    source,
    repo,
    onDiscard,
    onOpenFile,
    onToggleMarkdownPreview,
    markdownPreview,
    onToggleHtmlPreview,
    htmlPreview,
    showHtmlPreview,
  } = props;
  return (
    <>
      {onToggleMarkdownPreview && isMarkdownFile(filePath) && (
        <ToolbarIconBtn
          onClick={onToggleMarkdownPreview}
          tooltip={markdownPreview ? t("review:showDiff") : t("review:previewMarkdown")}
        >
          <IconEye className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      {showHtmlPreview && onToggleHtmlPreview && (
        <HtmlPreviewToolbarButton active={htmlPreview} onToggle={onToggleHtmlPreview} />
      )}
      {onOpenFile && (
        <ToolbarIconBtn onClick={() => onOpenFile(filePath, repo)} tooltip={t("common:edit")}>
          <IconPencil className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      <FileActionsDropdown
        filePath={filePath}
        sessionId={sessionId}
        repositoryId={repositoryId}
        repositoryName={repo}
        size="xs"
        includeCopyPath={false}
      />
      {source === "uncommitted" && (
        <ToolbarIconBtn
          onClick={onDiscard}
          tooltip={t("review:revertChanges")}
          className={iconBtnDestructive}
        >
          <IconArrowBackUp className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
    </>
  );
}

function DesktopFileDiffToolbar(props: FileDiffToolbarProps) {
  const { t } = useTranslation();
  const copyRepositoryPath = useCopyRepositoryPath();
  const {
    filePath,
    previousPath,
    status,
    taskId,
    sessionId,
    repositoryId,
    publishedBranch,
    baseBranch,
    wordWrap,
    expandUnchanged,
    onCommentFile,
    onToggleHtmlPreview,
    onToggleExpandUnchanged,
    onToggleWordWrap,
    repo,
  } = props;
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const showHtmlPreview = Boolean(onToggleHtmlPreview) && getFilePreviewKind(filePath) === "html";

  return (
    <div className="flex items-center gap-0.5">
      {onCommentFile && (
        <Button
          type="button"
          variant="ghost"
          onClick={onCommentFile}
          data-review-comment-opener
          aria-label={t("review:commentOnFile")}
          title={t("review:commentOnFile")}
          className="size-7 p-0 cursor-pointer [@media(pointer:coarse)]:size-11"
        >
          <IconMessagePlus className="size-4" />
        </Button>
      )}
      <ToolbarIconBtn
        onClick={() => void copyRepositoryPath(filePath)}
        tooltip={t("task:copyPath")}
      >
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
        onToggleViewMode={() => setGlobalViewMode(globalViewMode === "split" ? "unified" : "split")}
        onToggleWordWrap={onToggleWordWrap}
      />
      <DesktopFileActions {...props} showHtmlPreview={showHtmlPreview} />
    </div>
  );
}

export function FileDiffToolbar(props: FileDiffToolbarProps) {
  const { isMobile } = useResponsiveBreakpoint();
  return isMobile ? <MobileFileActionsMenu {...props} /> : <DesktopFileDiffToolbar {...props} />;
}
