"use client";

import { useEffect, useMemo, useRef, useState, type PointerEvent } from "react";
import { IconChevronLeft, IconChevronRight, IconGripVertical, IconList } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAzureDevOpsBoard } from "@/hooks/domains/azure-devops/use-azure-devops-board";
import type { AzureDevOpsBoardPreference } from "@/hooks/domains/azure-devops/use-azure-devops-preferences";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  AzureDevOpsActionPreset,
  AzureDevOpsBoard,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsProject,
  AzureDevOpsWorkItem,
} from "@/lib/types/azure-devops";
import { groupAzureDevOpsBoardItems } from "./azure-devops-board-view";
import { AzureDevOpsWorkItemDetail } from "./azure-devops-work-item-detail";

type Props = {
  workspaceId?: string;
  projectId: string;
  projects: AzureDevOpsProject[];
  onProjectChange: (projectId: string) => void;
  initialPreference?: AzureDevOpsBoardPreference;
  onPreferenceChange: (preference: AzureDevOpsBoardPreference) => void;
  quickActions?: AzureDevOpsActionPreset[];
  onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
};

function BoardCard({
  item,
  onOpen,
  onDragStart,
}: {
  item: AzureDevOpsBoardWorkItem;
  onOpen: () => void;
  onDragStart: () => void;
}) {
  const pointerStart = useRef<{ x: number; y: number } | null>(null);
  const handlePointerDown = (event: PointerEvent<HTMLDivElement>) => {
    pointerStart.current = { x: event.clientX, y: event.clientY };
  };
  const handlePointerUp = (event: PointerEvent<HTMLDivElement>) => {
    const start = pointerStart.current;
    pointerStart.current = null;
    if (!start || Math.hypot(event.clientX - start.x, event.clientY - start.y) < 6) onOpen();
  };
  return (
    <Card
      draggable
      role="button"
      tabIndex={0}
      onDragStart={onDragStart}
      onPointerDown={handlePointerDown}
      onPointerUp={handlePointerUp}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      className="cursor-pointer gap-2 py-3 transition hover:border-primary/50"
      data-testid={`azure-board-card-${item.id}`}
    >
      <CardHeader className="px-3 py-0">
        <CardTitle className="flex items-start gap-2 text-sm font-medium">
          <IconGripVertical className="mt-0.5 hidden h-4 w-4 shrink-0 text-muted-foreground md:block" />
          <span className="line-clamp-2">{item.title || "Untitled work item"}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 px-3 text-xs text-muted-foreground">
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant="outline">#{item.id}</Badge>
          {item.type && <Badge variant="secondary">{item.type}</Badge>}
          {item.assignedTo && <span className="truncate">{item.assignedTo}</span>}
        </div>
        {!!item.tags?.length && <div className="truncate">{item.tags.join(" · ")}</div>}
      </CardContent>
    </Card>
  );
}

function MobileColumnNavigator({
  columns,
  selectedColumn,
  counts,
  onChange,
}: {
  columns: AzureDevOpsBoard["columns"];
  selectedColumn: string;
  counts: Map<string, AzureDevOpsBoardWorkItem[]>;
  onChange: (columnId: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const index = Math.max(
    0,
    columns.findIndex((column) => column.id === selectedColumn),
  );
  const column = columns[index];
  const select = (columnId: string) => {
    onChange(columnId);
    setOpen(false);
  };
  return (
    <div className="flex min-h-11 items-center gap-2">
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="h-11 w-11 cursor-pointer"
        aria-label="Previous board column"
        disabled={index === 0}
        onClick={() => onChange(columns[index - 1].id)}
      >
        <IconChevronLeft className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="outline"
        className="min-h-11 flex-1 cursor-pointer justify-between"
        data-testid="azure-board-column-picker"
        onClick={() => setOpen(true)}
      >
        <span>{column?.name ?? "Board column"}</span>
        <span>{counts.get(column?.id ?? "")?.length ?? 0}</span>
        <IconList className="h-4 w-4" />
      </Button>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="h-11 w-11 cursor-pointer"
        aria-label="Next board column"
        disabled={index >= columns.length - 1}
        onClick={() => onChange(columns[index + 1].id)}
      >
        <IconChevronRight className="h-4 w-4" />
      </Button>
      <Drawer open={open} onOpenChange={setOpen}>
        <DrawerContent className="max-h-[min(32rem,calc(100dvh-16px-env(safe-area-inset-bottom,0px)))]">
          <DrawerHeader>
            <DrawerTitle>Board columns</DrawerTitle>
          </DrawerHeader>
          <div className="min-h-0 overflow-y-auto p-4 pb-[env(safe-area-inset-bottom,0px)]">
            {columns.map((candidate) => (
              <Button
                key={candidate.id}
                type="button"
                variant={candidate.id === selectedColumn ? "default" : "outline"}
                className="mb-2 min-h-11 w-full cursor-pointer justify-between"
                data-testid={`azure-board-column-option-${candidate.id}`}
                onClick={() => select(candidate.id)}
              >
                <span>{candidate.name}</span>
                <Badge variant="secondary">{counts.get(candidate.id)?.length ?? 0}</Badge>
              </Button>
            ))}
          </div>
        </DrawerContent>
      </Drawer>
    </div>
  );
}

function BoardSelectors({
  projectId,
  projects,
  onProjectChange,
  teamId,
  teams,
  onTeamChange,
  boardId,
  boards,
  onBoardChange,
}: {
  projectId: string;
  projects: AzureDevOpsProject[];
  onProjectChange: (value: string) => void;
  teamId: string;
  teams: Array<{ id: string; name: string }>;
  onTeamChange: (value: string) => void;
  boardId: string;
  boards: Array<{ id: string; name: string }>;
  onBoardChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      <Select value={projectId} onValueChange={onProjectChange}>
        <SelectTrigger className="w-52" data-testid="azure-board-project-select">
          <SelectValue placeholder="Project" />
        </SelectTrigger>
        <SelectContent>
          {projects.map((project) => (
            <SelectItem key={project.id} value={project.id}>
              {project.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select value={teamId} onValueChange={onTeamChange} disabled={!teams.length}>
        <SelectTrigger className="w-52" data-testid="azure-board-team-select">
          <SelectValue placeholder="Team" />
        </SelectTrigger>
        <SelectContent>
          {teams.map((team) => (
            <SelectItem key={team.id} value={team.id}>
              {team.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select value={boardId} onValueChange={onBoardChange} disabled={!boards.length}>
        <SelectTrigger className="w-52" data-testid="azure-board-select">
          <SelectValue placeholder="Board" />
        </SelectTrigger>
        <SelectContent>
          {boards.map((board) => (
            <SelectItem key={board.id} value={board.id}>
              {board.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function preferredColumn(
  columns: AzureDevOpsBoard["columns"],
  selectedColumn: string,
  initialPreference: AzureDevOpsBoardPreference | undefined,
): string {
  if (columns.some((column) => column.id === selectedColumn)) return selectedColumn;
  if (columns.some((column) => column.id === initialPreference?.focusedColumnId)) {
    return initialPreference?.focusedColumnId ?? "";
  }
  return columns[0]?.id ?? "";
}

function useBoardPreference(
  board: ReturnType<typeof useAzureDevOpsBoard>,
  columns: AzureDevOpsBoard["columns"],
  initialPreference: AzureDevOpsBoardPreference | undefined,
  onPreferenceChange: (preference: AzureDevOpsBoardPreference) => void,
) {
  const [selectedColumn, setSelectedColumn] = useState("");
  const effectiveColumn = preferredColumn(columns, selectedColumn, initialPreference);

  useEffect(() => {
    if (effectiveColumn !== selectedColumn) setSelectedColumn(effectiveColumn);
  }, [effectiveColumn, selectedColumn]);

  useEffect(() => {
    if (!board.teamId || !board.boardId || !effectiveColumn) return;
    onPreferenceChange({
      teamId: board.teamId,
      boardId: board.boardId,
      focusedColumnId: effectiveColumn,
    });
  }, [board.boardId, board.teamId, effectiveColumn, onPreferenceChange]);

  return { effectiveColumn, setSelectedColumn };
}

function useBoardGroups(snapshot: ReturnType<typeof useAzureDevOpsBoard>["snapshot"]) {
  return useMemo(
    () =>
      snapshot
        ? groupAzureDevOpsBoardItems(snapshot.board, snapshot.items)
        : new Map<string, AzureDevOpsBoardWorkItem[]>(),
    [snapshot],
  );
}

function useBoardPresentation(
  board: ReturnType<typeof useAzureDevOpsBoard>,
  initialPreference: AzureDevOpsBoardPreference | undefined,
  onPreferenceChange: (preference: AzureDevOpsBoardPreference) => void,
) {
  const groups = useBoardGroups(board.snapshot);
  const columns = board.snapshot?.board.columns ?? [];
  const columnPreference = useBoardPreference(
    board,
    columns,
    initialPreference,
    onPreferenceChange,
  );
  return { groups, columns, columnPreference };
}

function BoardEmptyState() {
  return (
    <div className="p-6 text-sm text-muted-foreground">Select a project to view its board.</div>
  );
}

// eslint-disable-next-line max-lines-per-function -- board rendering coordinates drag/drop, responsive columns, and detail state.
export function AzureDevOpsBoard({
  workspaceId,
  projectId,
  projects,
  onProjectChange,
  initialPreference,
  onPreferenceChange,
  quickActions = [],
  onStartTask,
}: Props) {
  const { isMobile } = useResponsiveBreakpoint();
  const board = useAzureDevOpsBoard(workspaceId, projectId, initialPreference);
  const [editing, setEditing] = useState<AzureDevOpsBoardWorkItem | null>(null);
  const [dragged, setDragged] = useState<AzureDevOpsBoardWorkItem | null>(null);
  const presentation = useBoardPresentation(board, initialPreference, onPreferenceChange);
  if (!projectId) return <BoardEmptyState />;
  return (
    <section
      className="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden p-4"
      data-testid="azure-devops-board"
    >
      <BoardSelectors
        projectId={projectId}
        projects={projects}
        onProjectChange={onProjectChange}
        teamId={board.teamId}
        teams={board.teams}
        onTeamChange={board.setTeamId}
        boardId={board.boardId}
        boards={board.boards}
        onBoardChange={board.setBoardId}
      />
      {board.error && (
        <Alert variant="destructive">
          <AlertDescription>{board.error}</AlertDescription>
        </Alert>
      )}
      {board.loading && <div className="p-6 text-sm text-muted-foreground">Loading board…</div>}
      {!board.loading && board.snapshot && (
        <>
          {isMobile && (
            <MobileColumnNavigator
              columns={presentation.columns}
              selectedColumn={presentation.columnPreference.effectiveColumn}
              counts={presentation.groups}
              onChange={presentation.columnPreference.setSelectedColumn}
            />
          )}
          <div
            className={
              isMobile
                ? "min-h-0 flex-1 overflow-y-auto"
                : "grid min-h-0 flex-1 auto-cols-fr grid-flow-col gap-3 overflow-x-auto"
            }
          >
            {presentation.columns
              .filter(
                (column) =>
                  !isMobile || column.id === presentation.columnPreference.effectiveColumn,
              )
              .map((column) => (
                <div
                  key={column.id}
                  data-testid={`azure-board-column-${column.id}`}
                  className="flex min-w-72 flex-col gap-2 rounded-lg bg-muted/40 p-2"
                  onDragOver={(event) => event.preventDefault()}
                  onDrop={() => {
                    if (dragged) void board.moveItem(dragged, column.id);
                    setDragged(null);
                  }}
                >
                  <div className="flex items-center justify-between px-1 text-sm font-semibold">
                    <span>{column.name}</span>
                    <Badge variant="secondary">
                      {presentation.groups.get(column.id)?.length ?? 0}
                    </Badge>
                  </div>
                  <div className="space-y-2">
                    {(presentation.groups.get(column.id) ?? []).map((item) => (
                      <BoardCard
                        key={item.id}
                        item={item}
                        onOpen={() => setEditing(item)}
                        onDragStart={() => setDragged(item)}
                      />
                    ))}
                  </div>
                </div>
              ))}
          </div>
          <AzureDevOpsWorkItemDetail
            open={!!editing}
            workspaceId={workspaceId}
            projectId={projectId}
            initialItem={editing}
            onOpenChange={(open) => !open && setEditing(null)}
            boardContext={
              editing
                ? {
                    board: board.snapshot.board,
                    item: editing,
                    onMove: board.moveItem,
                    onItemUpdated: (item) => {
                      board.mergeItem(item);
                      setEditing(item);
                      if (isMobile) {
                        presentation.columnPreference.setSelectedColumn(item.columnId);
                      }
                    },
                  }
                : undefined
            }
            quickActions={quickActions}
            onStartTask={onStartTask}
          />
        </>
      )}
    </section>
  );
}
