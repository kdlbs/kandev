"use client";

import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import rehypeSanitize from "rehype-sanitize";
import { IconExternalLink, IconPlus, IconRefresh, IconX } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAzureDevOpsWorkItemDetail } from "@/hooks/domains/azure-devops/use-azure-devops-work-item-detail";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type {
  AzureDevOpsActionPreset,
  AzureDevOpsBoard,
  AzureDevOpsBoardWorkItem,
  AzureDevOpsWorkItem,
} from "@/lib/types/azure-devops";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";

type BoardContext = {
  board: AzureDevOpsBoard;
  item: AzureDevOpsBoardWorkItem;
  onMove: (
    item: AzureDevOpsBoardWorkItem,
    columnId: string,
    columnDone?: boolean,
  ) => Promise<AzureDevOpsBoardWorkItem | undefined>;
  onItemUpdated?: (item: AzureDevOpsBoardWorkItem) => void;
};

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId?: string;
  projectId: string;
  initialItem: AzureDevOpsWorkItem | null;
  boardContext?: BoardContext;
  quickActions?: AzureDevOpsActionPreset[];
  onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
};

export function azureDevOpsDetailFields(item: AzureDevOpsWorkItem): Array<[string, string]> {
  return [
    ["Type", item.type || "Work item"],
    ["State", item.state || "Unknown"],
    ["Assignee", item.assignedTo || "Unassigned"],
    ...(item.areaPath ? [["Area", item.areaPath] as [string, string]] : []),
  ];
}

function Description({ value }: { value: string }) {
  if (!value.trim()) return <p className="text-sm text-muted-foreground">No description.</p>;
  return (
    <div
      data-testid="azure-work-item-detail-description"
      className="markdown-body max-w-none text-sm"
    >
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={[rehypeRaw, rehypeSanitize]}
        components={markdownComponents}
      >
        {value}
      </ReactMarkdown>
    </div>
  );
}

function AssignmentActions({
  assignedTo,
  saving,
  onChange,
}: {
  assignedTo?: string;
  saving: boolean;
  onChange: (action: "assign_current_user" | "unassign") => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        type="button"
        size="sm"
        className="min-h-11 cursor-pointer"
        disabled={saving}
        onClick={() => onChange("assign_current_user")}
        data-testid="azure-work-item-assign-current-user"
      >
        {saving ? "Updating…" : "Assign to me"}
      </Button>
      {assignedTo && (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer"
          disabled={saving}
          onClick={() => onChange("unassign")}
          data-testid="azure-work-item-unassign"
        >
          Unassign
        </Button>
      )}
    </div>
  );
}

function BoardActions({
  context,
  saving,
  onMove,
}: {
  context: BoardContext;
  saving: boolean;
  onMove: (
    item: AzureDevOpsBoardWorkItem,
    columnId: string,
    columnDone?: boolean,
  ) => Promise<AzureDevOpsBoardWorkItem | undefined>;
}) {
  const [currentItem, setCurrentItem] = useState(context.item);
  const [columnId, setColumnId] = useState(context.item.columnId);
  const [columnDone, setColumnDone] = useState(context.item.columnDone);
  useEffect(() => {
    setCurrentItem(context.item);
    setColumnId(context.item.columnId);
    setColumnDone(context.item.columnDone);
  }, [context.item.columnDone, context.item.columnId, context.item.id, context.item.revision]);
  const selected = context.board.columns.find((column) => column.id === columnId);
  return (
    <div className="space-y-2" data-testid="azure-work-item-board-actions">
      <Label htmlFor="azure-work-item-column">Move to column</Label>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Select value={columnId} onValueChange={setColumnId}>
          <SelectTrigger
            id="azure-work-item-column"
            className="min-h-11 flex-1"
            data-testid="azure-work-item-column"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {context.board.columns.map((column) => (
              <SelectItem key={column.id} value={column.id}>
                {column.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {selected?.isSplit && (
          <Select
            value={String(columnDone)}
            onValueChange={(value) => setColumnDone(value === "true")}
          >
            <SelectTrigger className="min-h-11 sm:w-40" data-testid="azure-work-item-column-done">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="false">In progress</SelectItem>
              <SelectItem value="true">Done</SelectItem>
            </SelectContent>
          </Select>
        )}
        <Button
          type="button"
          className="min-h-11 cursor-pointer"
          disabled={
            saving || (columnId === context.item.columnId && columnDone === context.item.columnDone)
          }
          onClick={() =>
            void onMove(currentItem, columnId, columnDone).then((updated) => {
              if (updated) setCurrentItem(updated);
            })
          }
        >
          Move
        </Button>
      </div>
    </div>
  );
}

function QuickActions({
  actions,
  item,
  onStartTask,
}: {
  actions: AzureDevOpsActionPreset[];
  item: AzureDevOpsWorkItem;
  onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
}) {
  if (!onStartTask || actions.length === 0) return null;
  return (
    <div className="space-y-2" data-testid="azure-work-item-quick-actions">
      <Label>Task actions</Label>
      <div className="flex flex-wrap gap-2">
        {actions.map((action) => (
          <Button
            key={action.id}
            type="button"
            variant="outline"
            className="min-h-11 cursor-pointer"
            onClick={() => onStartTask(item, action)}
          >
            <IconPlus className="h-4 w-4" />
            {action.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

// eslint-disable-next-line max-lines-per-function, complexity -- the detail body presents the complete read-only work-item surface and supported actions.
function DetailBody({
  state,
  boardContext,
  quickActions,
  onStartTask,
}: {
  state: ReturnType<typeof useAzureDevOpsWorkItemDetail>;
  boardContext?: BoardContext;
  quickActions: AzureDevOpsActionPreset[];
  onStartTask?: (item: AzureDevOpsWorkItem, action?: AzureDevOpsActionPreset) => void;
}) {
  const [saving, setSaving] = useState(false);
  const [mutationError, setMutationError] = useState<string | null>(null);
  const item = state.item;
  if (!item) {
    return (
      <div className="flex min-h-48 items-center justify-center text-sm text-muted-foreground">
        {state.loading ? "Loading work item…" : "Work item unavailable."}
      </div>
    );
  }
  const planningFields = item.planningFields ?? [];
  const updateAssignee = async (action: "assign_current_user" | "unassign") => {
    setSaving(true);
    setMutationError(null);
    try {
      const updated = await state.updateAssignee(action);
      if (updated && boardContext?.onItemUpdated) {
        boardContext.onItemUpdated({ ...boardContext.item, ...updated });
      }
    } catch (error) {
      setMutationError(String(error));
    } finally {
      setSaving(false);
    }
  };
  return (
    <div
      className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-4 sm:px-6"
      data-testid="azure-work-item-detail-body"
    >
      {state.error && (
        <Alert variant="destructive">
          <AlertDescription className="flex items-center justify-between gap-2">
            {state.error}
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="min-h-11 cursor-pointer"
              onClick={() => void state.refresh()}
            >
              <IconRefresh className="h-4 w-4" /> Retry
            </Button>
          </AlertDescription>
        </Alert>
      )}
      {mutationError && (
        <Alert variant="destructive">
          <AlertDescription>{mutationError}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        {azureDevOpsDetailFields(item).map(([label, value]) => (
          <div key={label} className="rounded-md border p-3">
            <div className="text-xs text-muted-foreground">{label}</div>
            <div className="break-words text-sm font-medium">{value}</div>
          </div>
        ))}
      </div>
      {!!item.tags?.length && (
        <div className="flex flex-wrap gap-1.5">
          {item.tags.map((tag) => (
            <Badge key={tag} variant="secondary">
              {tag}
            </Badge>
          ))}
        </div>
      )}
      <section className="space-y-2">
        <h3 className="text-sm font-semibold">Description</h3>
        <Description value={item.description ?? ""} />
      </section>
      {planningFields.length > 0 && (
        <section className="space-y-2">
          <h3 className="text-sm font-semibold">Planning and effort</h3>
          <div className="grid gap-2 sm:grid-cols-2">
            {planningFields.map((field) => (
              <div key={field.referenceName} className="rounded-md bg-muted/40 p-3 text-sm">
                <div className="text-xs text-muted-foreground">{field.label}</div>
                <div>{field.value || "—"}</div>
              </div>
            ))}
          </div>
        </section>
      )}
      <section className="space-y-2">
        <h3 className="text-sm font-semibold">Assignment</h3>
        <AssignmentActions
          assignedTo={item.assignedTo}
          saving={saving}
          onChange={(action) => void updateAssignee(action)}
        />
      </section>
      {boardContext && (
        <BoardActions
          context={boardContext}
          saving={saving}
          onMove={async (item, columnId, columnDone) => {
            setSaving(true);
            setMutationError(null);
            try {
              const updated = await boardContext.onMove(item, columnId, columnDone);
              if (updated) {
                state.mergeItem(updated);
                boardContext.onItemUpdated?.(updated);
              }
              return updated;
            } catch (error) {
              setMutationError(String(error));
              return undefined;
            } finally {
              setSaving(false);
            }
          }}
        />
      )}
      <QuickActions actions={quickActions} item={item} onStartTask={onStartTask} />
      <section className="space-y-2" data-testid="azure-work-item-detail-comments">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-semibold">Discussion</h3>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="min-h-11 cursor-pointer"
            onClick={() => void state.retryComments()}
            disabled={state.commentsLoading}
          >
            <IconRefresh className="h-4 w-4" /> Retry
          </Button>
        </div>
        {state.commentsError && <p className="text-sm text-destructive">{state.commentsError}</p>}
        {state.comments.map((comment) => (
          <article key={comment.id} className="rounded-md border p-3">
            <div className="mb-1 text-xs text-muted-foreground">
              {comment.author.displayName || comment.author.uniqueName || "Azure DevOps"}
            </div>
            <div className="whitespace-pre-wrap text-sm">{comment.content}</div>
          </article>
        ))}
        {state.continuationToken && (
          <Button
            type="button"
            variant="outline"
            className="min-h-11 w-full cursor-pointer"
            onClick={state.loadOlderComments}
            disabled={state.commentsLoading}
          >
            {state.commentsLoading ? "Loading…" : "Load older discussion"}
          </Button>
        )}
        {!state.commentsLoading && !state.commentsError && state.comments.length === 0 && (
          <p className="text-sm text-muted-foreground">No discussion yet.</p>
        )}
      </section>
      <section className="space-y-2">
        <h3 className="text-sm font-semibold">Linked Kandev tasks</h3>
        {state.linkedTasksLoading && (
          <p className="text-sm text-muted-foreground">Loading linked tasks…</p>
        )}
        {!state.linkedTasksLoading && state.linkedTasks.length === 0 && (
          <p className="text-sm text-muted-foreground">No linked tasks.</p>
        )}
        {state.linkedTasks.map((task) => (
          <div key={task.id} className="rounded-md border p-3 text-sm">
            {task.title}
          </div>
        ))}
      </section>
    </div>
  );
}

// eslint-disable-next-line complexity -- the component switches between the desktop dialog and mobile drawer while preserving one detail state.
export function AzureDevOpsWorkItemDetail({
  open,
  onOpenChange,
  workspaceId,
  projectId,
  initialItem,
  boardContext,
  quickActions = [],
  onStartTask,
}: Props) {
  const { isMobile } = useResponsiveBreakpoint();
  const state = useAzureDevOpsWorkItemDetail(workspaceId, projectId, initialItem, open);
  const title = state.item?.title ?? initialItem?.title ?? "Work item";
  const header = (
    <div className="flex items-start justify-between gap-3 border-b px-4 py-4 sm:px-6">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="outline">#{state.item?.id ?? initialItem?.id}</Badge>
          <Badge variant="secondary">{state.item?.type ?? initialItem?.type}</Badge>
          <span>{state.item?.state ?? initialItem?.state}</span>
        </div>
        <h2 className="mt-1 break-words text-lg font-semibold">{title}</h2>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {(state.item?.webUrl ?? initialItem?.webUrl) && (
          <Button
            asChild
            type="button"
            variant="ghost"
            size="icon"
            className="min-h-11 min-w-11 cursor-pointer"
          >
            <a
              href={state.item?.webUrl ?? initialItem?.webUrl}
              target="_blank"
              rel="noreferrer"
              aria-label="Open work item in Azure DevOps"
            >
              <IconExternalLink className="h-4 w-4" />
            </a>
          </Button>
        )}
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="min-h-11 min-w-11 cursor-pointer"
          aria-label="Close work item details"
          data-testid="azure-work-item-detail-close"
          onClick={() => onOpenChange(false)}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
  const body = (
    <DetailBody
      state={state}
      boardContext={boardContext}
      quickActions={quickActions}
      onStartTask={onStartTask}
    />
  );
  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="h-[100dvh] max-h-[100dvh] overflow-hidden pb-[env(safe-area-inset-bottom,0px)]">
          <DrawerHeader className="sr-only">
            <DrawerTitle>{title}</DrawerTitle>
          </DrawerHeader>
          <div data-testid={open ? "azure-work-item-detail" : undefined} className="contents">
            {header}
            {body}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90dvh] w-[min(900px,95vw)] flex-col gap-0 overflow-hidden p-0">
        <DialogHeader className="sr-only">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>Azure DevOps work-item details</DialogDescription>
        </DialogHeader>
        <div data-testid={open ? "azure-work-item-detail" : undefined} className="contents">
          {header}
          {body}
        </div>
      </DialogContent>
    </Dialog>
  );
}
