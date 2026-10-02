"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Skeleton } from "@kandev/ui/skeleton";
import { Switch } from "@kandev/ui/switch";
import {
  MAX_WATCHED_BOARDS,
  switchOffWatches,
  type WatchesDraft,
} from "@/lib/coordinators/control-draft";
import {
  useWorkspaceBoards,
  type BoardsStatus,
  type WorkspaceBoard,
} from "@/hooks/domains/coordinator/use-workspace-boards";
import type { useControlDraft } from "@/hooks/domains/coordinator/use-control-draft";
import { ProjectsFields, type ProjectsFieldsProps } from "./projects-fields";

type Control = ReturnType<typeof useControlDraft>;

type WatchesSectionProps = {
  workspaceId: string;
  canManage: boolean;
  control: Control;
};

type BoardRowProps = {
  board: WorkspaceBoard;
  inScope: boolean;
  canManage: boolean;
  disabledAdd: boolean;
  disabledRemove: boolean;
  onToggle: () => void;
};

function BoardRow({
  board,
  inScope,
  canManage,
  disabledAdd,
  disabledRemove,
  onToggle,
}: BoardRowProps) {
  const { t } = useTranslation();
  const disabled = !canManage || (inScope ? disabledRemove : disabledAdd);
  return (
    <li
      className="flex items-center justify-between gap-2 py-2"
      data-testid={`watches-board-${board.id}`}
    >
      <span className="flex items-center gap-2 text-sm">
        {board.name}
        {board.hidden && <Badge variant="outline">{t("coordinator:watchesHidden")}</Badge>}
        {inScope && (
          <span className="text-xs text-muted-foreground">{t("coordinator:watchesInScope")}</span>
        )}
      </span>
      <Button
        variant="outline"
        size="sm"
        className="cursor-pointer"
        disabled={disabled}
        onClick={onToggle}
        data-testid={`watches-toggle-${board.id}`}
      >
        {inScope ? t("coordinator:watchesTakeOut") : t("coordinator:watchesPutIn")}
      </Button>
    </li>
  );
}

export type WatchesFieldsProps = {
  watches: WatchesDraft;
  boards: WorkspaceBoard[];
  boardsStatus: BoardsStatus;
  onBoardsRetry: () => void;
  canManage: boolean;
  onChange: (next: WatchesDraft) => void;
  errorMessage?: string | null;
  /** The Projects part; absent while the Projects scope is not offered. */
  projects?: ProjectsFieldsProps;
};

export function WatchesFields({
  watches,
  boards,
  boardsStatus,
  onBoardsRetry,
  canManage,
  onChange,
  errorMessage,
  projects,
}: WatchesFieldsProps) {
  const { t } = useTranslation();
  const { scope, workflowIds } = watches;
  const selected = new Set(workflowIds);
  const atCap = workflowIds.length >= MAX_WATCHED_BOARDS;
  const lastBoard = workflowIds.length <= 1;

  const toggleAll = (watchAll: boolean) => {
    if (watchAll) return onChange({ scope: "all", workflowIds });
    const off = switchOffWatches(boards.map((b) => b.id));
    if (off.ok) onChange(off.watches);
  };
  const toggleBoard = (id: string) => {
    const next = selected.has(id) ? workflowIds.filter((x) => x !== id) : [...workflowIds, id];
    onChange({ scope: "selected", workflowIds: next });
  };

  const noBoards = boardsStatus === "ready" && boards.length === 0;
  const cannotSwitchOff = scope === "all" && (boardsStatus !== "ready" || noBoards);
  return (
    <div className="space-y-3" data-testid="watches-section">
      <div className="flex items-center justify-between gap-2">
        <label htmlFor="watches-all" className="cursor-pointer text-sm font-medium">
          {t("coordinator:watchesAll")}
        </label>
        <Switch
          id="watches-all"
          checked={scope === "all"}
          disabled={!canManage || cannotSwitchOff}
          onCheckedChange={toggleAll}
        />
      </div>
      {errorMessage && (
        <p role="alert" className="text-sm text-destructive" data-testid="watches-error">
          {errorMessage}
        </p>
      )}
      {scope === "all" && (
        <p className="text-xs text-muted-foreground">{t("coordinator:watchesAllHelp")}</p>
      )}
      {noBoards && (
        <p className="text-sm text-muted-foreground" data-testid="watches-no-boards">
          {t("coordinator:watchesNoBoardsToChoose")}
        </p>
      )}
      {boardsStatus === "error" && (
        <div className="flex items-center gap-2 text-sm" data-testid="watches-boards-failed">
          {t("coordinator:watchesBoardsFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={onBoardsRetry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      )}
      {scope === "selected" && boards.length > 0 && (
        <>
          <ul className="divide-y">
            {boards.map((board) => (
              <BoardRow
                key={board.id}
                board={board}
                inScope={selected.has(board.id)}
                canManage={canManage}
                disabledAdd={atCap}
                disabledRemove={lastBoard}
                onToggle={() => toggleBoard(board.id)}
              />
            ))}
          </ul>
          {atCap && (
            <p className="text-xs text-muted-foreground">{t("coordinator:watchesAtMost")}</p>
          )}
          {lastBoard && workflowIds.length === 1 && (
            <p className="text-xs text-muted-foreground">{t("coordinator:watchesKeepOneBoard")}</p>
          )}
        </>
      )}
      {projects && <ProjectsFields {...projects} />}
    </div>
  );
}

export function WatchesSection({ workspaceId, canManage, control }: WatchesSectionProps) {
  const { t } = useTranslation();
  const { draft, status, retry, setWatches, setProjects, projectsRead } = control;
  const boardsRead = useWorkspaceBoards(workspaceId, draft !== null);

  if (!draft) {
    if (status === "error") {
      return (
        <div className="flex items-center gap-2 text-sm" data-testid="watches-load-failed">
          {t("coordinator:watchesLoadFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={retry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      );
    }
    return <Skeleton className="h-32 w-full" data-testid="watches-loading" />;
  }

  return (
    <WatchesFields
      watches={draft.watches}
      boards={boardsRead.boards}
      boardsStatus={boardsRead.status}
      onBoardsRetry={boardsRead.retry}
      canManage={canManage}
      onChange={setWatches}
      projects={
        draft.projects
          ? {
              projects: draft.projects,
              sets: projectsRead.sets,
              loose: projectsRead.loose,
              status: projectsRead.status,
              onRetry: projectsRead.retry,
              canManage,
              onChange: setProjects,
            }
          : undefined
      }
    />
  );
}
