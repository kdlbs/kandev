"use client";

import { IconExternalLink, IconPlus, IconRefresh } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import type { LinearIssue, LinearWorkflowState } from "@/lib/types/linear";
import {
  LinearErrorMessage,
  PersonCell,
  formatRelative,
  priorityClass,
  stateBadgeClass,
  useIssueState,
  type IssueState,
} from "./linear-issue-common";

type LinearIssueDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null | undefined;
  identifier: string | null | undefined;
  /** Shown while the full issue is loading so the dialog doesn't flash empty. */
  initialIssue?: LinearIssue | null;
  /** When set, the dialog footer shows a Start task button that calls this. */
  onStartTask?: (issue: LinearIssue) => void;
};

function dialogTitle(issue: LinearIssue | null, identifier: string | null | undefined): string {
  return issue?.title ?? identifier ?? "Linear issue";
}

function effectiveIdentifier(
  issue: LinearIssue | null,
  identifier: string | null | undefined,
): string {
  return identifier ?? issue?.identifier ?? "";
}

export function LinearIssueDialog({
  open,
  onOpenChange,
  workspaceId,
  identifier,
  initialIssue,
  onStartTask,
}: LinearIssueDialogProps) {
  const enabled = open && !!workspaceId && !!identifier;
  const state = useIssueState(workspaceId ?? "", identifier ?? "", enabled);
  const issue = state.issue ?? initialIssue ?? null;

  const errorOnly = state.error !== null && issue === null;
  const showFooter = issue !== null && onStartTask !== undefined;

  // The "Start task" button hands the dialog's issue back to the caller and
  // closes — the caller is responsible for opening the task creation flow.
  const handleStart = () => {
    if (issue && onStartTask) {
      onStartTask(issue);
      onOpenChange(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="!max-w-[min(1280px,95vw)] w-[95vw] max-h-[90vh] overflow-hidden flex flex-col gap-0 p-0 sm:rounded-lg">
        <DialogTitle className="sr-only">{dialogTitle(issue, identifier)}</DialogTitle>
        {errorOnly ? (
          <div className="flex-1 flex items-center justify-center px-8 py-16">
            <LinearErrorMessage error={state.error ?? ""} workspaceId={workspaceId} />
          </div>
        ) : (
          <>
            <IssueTopBar
              issue={issue}
              loading={state.loading}
              onRefresh={() => void state.load()}
            />
            <DialogBody
              issue={issue}
              state={state}
              identifier={effectiveIdentifier(issue, identifier)}
              workspaceId={workspaceId}
            />
            {showFooter && <IssueFooter onStart={handleStart} />}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function IssueFooter({ onStart }: { onStart: () => void }) {
  return (
    <div className="flex items-center justify-end gap-2 px-6 py-3 border-t bg-muted/20 shrink-0">
      <Button size="sm" variant="default" className="cursor-pointer gap-1.5" onClick={onStart}>
        <IconPlus className="h-4 w-4" />
        Start task
      </Button>
    </div>
  );
}

function DialogBody({
  issue,
  state,
  identifier,
  workspaceId,
}: {
  issue: LinearIssue | null;
  state: IssueState;
  identifier: string;
  workspaceId: string | null | undefined;
}) {
  return (
    <div className="flex-1 overflow-y-auto">
      {state.error && issue && (
        <div className="px-8 pt-4">
          <LinearErrorMessage error={state.error} workspaceId={workspaceId} compact />
        </div>
      )}
      {!issue && state.loading && (
        <div className="text-sm text-muted-foreground py-16 text-center">Loading issue…</div>
      )}
      {issue && <IssueBody issue={issue} state={state} identifier={identifier} />}
    </div>
  );
}

type TopBarProps = {
  issue: LinearIssue | null;
  loading: boolean;
  onRefresh: () => void;
};

function IssueTopBar({ issue, loading, onRefresh }: TopBarProps) {
  return (
    <div className="flex items-center justify-end gap-1 pl-4 pr-12 py-2 border-b shrink-0">
      {issue?.url && (
        <Button
          asChild
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer"
          title="Open in Linear"
        >
          <a href={issue.url} target="_blank" rel="noreferrer">
            <IconExternalLink className="h-4 w-4" />
          </a>
        </Button>
      )}
      <Button
        variant="ghost"
        size="icon-sm"
        className="cursor-pointer"
        onClick={onRefresh}
        disabled={loading}
        title="Refresh"
      >
        <IconRefresh className={`h-4 w-4 ${loading ? "animate-spin" : ""}`} />
      </Button>
    </div>
  );
}

function IssueBody({
  issue,
  state,
  identifier,
}: {
  issue: LinearIssue;
  state: IssueState;
  identifier: string;
}) {
  return (
    <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_340px] gap-8 px-8 py-6">
      <div className="min-w-0 space-y-6">
        <IssueHeading issue={issue} identifier={identifier} />
        <DescriptionSection description={issue.description} />
      </div>
      <aside className="space-y-4">
        <StateBlock
          issue={issue}
          states={issue.states}
          pending={state.pendingState}
          onChange={(id) => void state.handleStateChange(id)}
        />
        <DetailsCard issue={issue} />
        <MetaFooter issue={issue} />
      </aside>
    </div>
  );
}

function IssueHeading({ issue, identifier }: { issue: LinearIssue; identifier: string }) {
  return (
    <div className="space-y-3">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground font-mono">
        <span>{identifier}</span>
        {issue.teamKey && (
          <>
            <span>·</span>
            <span>{issue.teamKey}</span>
          </>
        )}
      </div>
      <h2 className="text-2xl font-semibold leading-tight">{issue.title}</h2>
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return <div className="text-sm font-semibold">{children}</div>;
}

function DescriptionSection({ description }: { description: string }) {
  return (
    <section className="space-y-2">
      <SectionHeading>Description</SectionHeading>
      <div className="rounded-md border bg-muted/30 px-4 py-3">
        {description ? (
          <div className="text-sm whitespace-pre-wrap leading-relaxed">{description}</div>
        ) : (
          <div className="text-sm text-muted-foreground italic">No description.</div>
        )}
      </div>
    </section>
  );
}

type StateBlockProps = {
  issue: LinearIssue;
  states: LinearWorkflowState[] | null | undefined;
  pending: string | null;
  onChange: (stateId: string) => void;
};

// Hide the issue's current state from the available transitions list — moving
// to the same state would be a no-op and clutters the UI.
function visibleStates(issue: LinearIssue, list: LinearWorkflowState[]): LinearWorkflowState[] {
  return list.filter((s) => s.id !== issue.stateId);
}

function StateBlock({ issue, states, pending, onChange }: StateBlockProps) {
  const list = states ?? [];
  const next = visibleStates(issue, list);
  return (
    <section className="space-y-2">
      {issue.stateName && (
        <div>
          <Badge
            variant="outline"
            className={`text-sm px-3 py-1 ${stateBadgeClass(issue.stateCategory)}`}
          >
            {issue.stateName}
          </Badge>
        </div>
      )}
      {next.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pt-1">
          {next.map((s) => (
            <Button
              key={s.id}
              variant="outline"
              size="sm"
              className="cursor-pointer h-7 text-xs"
              disabled={pending !== null}
              onClick={() => onChange(s.id)}
            >
              {pending === s.id ? "…" : `→ ${s.name}`}
            </Button>
          ))}
        </div>
      )}
    </section>
  );
}

function DetailsCard({ issue }: { issue: LinearIssue }) {
  return (
    <section className="rounded-md border bg-background overflow-hidden">
      <div className="px-4 py-3 border-b bg-muted/30">
        <SectionHeading>Details</SectionHeading>
      </div>
      <div className="px-4 py-3 space-y-3 text-sm">
        <DetailRow label="Assignee">
          <PersonCell name={issue.assigneeName} avatar={issue.assigneeIcon} />
        </DetailRow>
        <DetailRow label="Creator">
          <PersonCell name={issue.creatorName} avatar={issue.creatorIcon} />
        </DetailRow>
        <DetailRow label="Priority">
          <span className={priorityClass(issue.priority)}>{issue.priorityLabel || "—"}</span>
        </DetailRow>
        <DetailRow label="Team">
          <span className="font-mono text-xs">{issue.teamKey}</span>
        </DetailRow>
      </div>
    </section>
  );
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-3">
      <span className="text-muted-foreground text-xs w-20 shrink-0 pt-0.5">{label}</span>
      <div className="min-w-0 flex items-center gap-1.5 flex-1">{children}</div>
    </div>
  );
}

function MetaFooter({ issue }: { issue: LinearIssue }) {
  const updated = formatRelative(issue.updated);
  if (!updated) return null;
  return (
    <div className="text-xs text-muted-foreground px-1" title={issue.updated}>
      Updated {updated}
    </div>
  );
}
