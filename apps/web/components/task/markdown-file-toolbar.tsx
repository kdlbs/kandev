import type { ReactNode } from "react";
import { IconDeviceFloppy, IconLoader2, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { FileViewerExternalLink } from "./file-viewer-header";
import { PanelHeaderBarSplit } from "./panel-primitives";
import { toRelativePath } from "@/lib/utils";
import { useTranslation } from "react-i18next";

const SAVE_SHORTCUT =
  typeof navigator !== "undefined" && navigator.platform.includes("Mac") ? "\u2318" : "Ctrl";

type MarkdownFileActionsProps = {
  isDirty: boolean;
  isSaving: boolean;
  hasRemoteUpdate: boolean;
  onSave: () => void;
  onReloadFromAgent?: () => void;
  onDelete?: () => void;
};

export function MarkdownFileActions({
  isDirty,
  isSaving,
  hasRemoteUpdate,
  onSave,
  onReloadFromAgent,
  onDelete,
}: MarkdownFileActionsProps) {
  const { t } = useTranslation();
  return (
    <>
      {hasRemoteUpdate && onReloadFromAgent && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-6 cursor-pointer gap-1 px-2 text-xs"
              onClick={onReloadFromAgent}
              data-testid="markdown-file-reload"
            >
              <IconRefresh className="h-3.5 w-3.5" />
              {t("editors:reload")}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("editors:applyLatestAgentChangesToFile")}</TooltipContent>
        </Tooltip>
      )}
      {onDelete && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 w-6 cursor-pointer p-0 hover:text-destructive"
              onClick={onDelete}
              aria-label={t("editors:deleteFile")}
              data-testid="markdown-file-delete"
            >
              <IconTrash className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("editors:deleteFile")}</TooltipContent>
        </Tooltip>
      )}
      <Button
        type="button"
        variant="default"
        size="sm"
        className="h-6 cursor-pointer gap-1 px-2 text-xs"
        disabled={!isDirty || isSaving}
        onClick={onSave}
        data-testid="markdown-file-save"
      >
        {isSaving ? (
          <>
            <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
            {t("editors:saving")}
          </>
        ) : (
          <>
            <IconDeviceFloppy className="h-3.5 w-3.5" />
            {t("common:save")}
            <span className="text-xs">({SAVE_SHORTCUT}+S)</span>
          </>
        )}
      </Button>
    </>
  );
}

type MarkdownEditToolbarProps = {
  path: string;
  worktreePath?: string;
  modeControl: ReactNode;
  fileActions: ReactNode;
  sessionId?: string | null;
  taskId?: string | null;
  repositoryId?: string | null;
  repositoryName?: string;
};

export function MarkdownEditToolbar({
  path,
  worktreePath,
  modeControl,
  fileActions,
  sessionId,
  taskId,
  repositoryId,
  repositoryName,
}: MarkdownEditToolbarProps) {
  return (
    <PanelHeaderBarSplit
      className="markdown-file-toolbar"
      left={
        <ScrollOnOverflow className="min-w-0 font-mono text-xs text-muted-foreground">
          {toRelativePath(path, worktreePath)}
        </ScrollOnOverflow>
      }
      right={
        <div className="flex items-center gap-1">
          {modeControl}
          <FileViewerExternalLink
            path={path}
            sessionId={sessionId}
            taskId={taskId}
            repositoryId={repositoryId}
            repositoryName={repositoryName}
          />
          {fileActions}
        </div>
      }
    />
  );
}
