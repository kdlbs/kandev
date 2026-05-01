"use client";

import { Fragment, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  IconLoader2,
  IconAlertTriangle,
  IconInfoCircle,
  IconArrowRight,
  IconChevronDown,
  IconBrandGithub,
  IconGitFork,
  IconStethoscope,
} from "@tabler/icons-react";

import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";
import { cn } from "@/lib/utils";
import type { Task, WorkflowStep } from "@/lib/types/http";

export type BootstrapState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "ready"; data: ImproveKandevBootstrapResponse; steps: WorkflowStep[] }
  | { kind: "error"; message: string }
  | { kind: "blocked"; message: string };

type ImproveKind = "bug" | "feature";

const PLACEHOLDERS: Record<ImproveKind, string> = {
  bug: "E.g. When I open the kanban board, the column header text overlaps the task count badge on screens narrower than 1200px...",
  feature:
    "E.g. Add a keyboard shortcut to mark a task as Done from the task detail view, and show the shortcut in the task action menu...",
};

type CreateModeViewProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  bootstrap: BootstrapState;
  captureLogs: boolean;
  setCaptureLogs: (v: boolean) => void;
  transformDescription: (description: string) => Promise<string>;
  onTaskCreated: (task: Task) => void;
};

export function CreateModeView(props: CreateModeViewProps) {
  const { bootstrap, captureLogs, setCaptureLogs } = props;
  const [kind, setKind] = useState<ImproveKind>("bug");
  const ready = bootstrap.kind === "ready" ? bootstrap : null;
  const startStep = ready ? (ready.steps.find((s) => s.is_start_step) ?? ready.steps[0]) : null;

  const handleKindChange = (next: ImproveKind) => {
    setKind(next);
    setCaptureLogs(next === "bug");
  };

  // Hard-block the contribution flow if the bootstrap probe detected the
  // user can't fork kdlbs/kandev (e.g., EMU enterprise restriction). Showing
  // the task form here would only let them fill out a task that fails at
  // the PR step.
  if (bootstrap.kind === "blocked") {
    return (
      <BlockedDialog
        open={props.open}
        onOpenChange={props.onOpenChange}
        message={bootstrap.message}
      />
    );
  }

  return (
    <TaskCreateDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      mode="create"
      workspaceId={props.workspaceId}
      workflowId={ready?.data.workflow_id ?? null}
      defaultStepId={startStep?.id ?? null}
      steps={ready ? ready.steps.map((s) => ({ id: s.id, title: s.name, events: s.events })) : []}
      initialValues={{
        title: "",
        repositoryId: ready?.data.repository_id ?? "",
        branch: ready?.data.branch ?? "",
      }}
      onSuccess={props.onTaskCreated}
      lockedFields={{ repository: true, branch: true, workflow: true }}
      descriptionPlaceholder={PLACEHOLDERS[kind]}
      aboveDescriptionSlot={<KindTabs kind={kind} onChange={handleKindChange} />}
      extraFormSlot={
        <BootstrapStatusSlot
          bootstrap={bootstrap}
          captureLogs={captureLogs}
          setCaptureLogs={setCaptureLogs}
        />
      }
      bottomSlot={
        ready && (
          <div className="space-y-2">
            <WorkflowStepsPreview steps={ready.steps} />
            <UsefulInfoCollapsible />
          </div>
        )
      }
      transformDescriptionBeforeSubmit={props.transformDescription}
    />
  );
}

const STEP_DESCRIPTIONS: Record<string, string> = {
  improve:
    "Agent reads the report, explores the codebase, and implements the change with TDD. Runs make fmt, typecheck, test, lint, then commits.",
  test: "Agent boots a secondary kandev instance with make dev and tells you what to verify. You confirm the change works in the running app.",
  pr: "Agent runs the pr skill: pushes the branch and opens a pull request to kdlbs/kandev for the maintainers to review.",
};

function WorkflowStepsPreview({ steps }: { steps: WorkflowStep[] }) {
  if (steps.length === 0) return null;
  const ordered = [...steps].sort((a, b) => a.position - b.position);
  return (
    <div className="rounded-md border border-border bg-muted/30 px-3 py-2">
      <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground/80">
        Workflow
      </p>
      <TooltipProvider delayDuration={150}>
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
          {ordered.map((s, i) => (
            <Fragment key={s.id}>
              {i > 0 && <IconArrowRight className="h-3 w-3 shrink-0 text-muted-foreground/50" />}
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="flex cursor-help items-center gap-1.5">
                    <span
                      className={cn(
                        "h-1.5 w-1.5 rounded-full shrink-0",
                        s.color || "bg-muted-foreground",
                      )}
                    />
                    <span className="text-foreground">{s.name}</span>
                  </span>
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs text-xs leading-relaxed">
                  {STEP_DESCRIPTIONS[s.name.toLowerCase()] ?? s.name}
                </TooltipContent>
              </Tooltip>
            </Fragment>
          ))}
        </div>
      </TooltipProvider>
    </div>
  );
}

function UsefulInfoCollapsible() {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="group flex w-full cursor-pointer items-center justify-between rounded-md border border-border bg-muted/30 px-3 py-2 text-left text-xs hover:bg-muted/50"
        >
          <span className="font-medium text-muted-foreground">
            Useful commands &amp; agent skills
          </span>
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 space-y-3 rounded-md border border-border bg-muted/20 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
          <p>
            Shell command you can run in the secondary instance, plus slash-command <em>skills</em>{" "}
            you can ask the agent to run during the workflow.
          </p>
          <div className="space-y-2">
            <p className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground/70">
              Shell
            </p>
            <UsefulInfoItem cmd="make install && make dev">
              Boots a secondary kandev dev instance with a clean DB so you can verify the change
              without touching your main one.
            </UsefulInfoItem>
          </div>
          <div className="space-y-2">
            <p className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground/70">
              Agent skills (ask the agent to run)
            </p>
            <UsefulInfoItem cmd="/commit">
              Stages and commits changes using Conventional Commits.
            </UsefulInfoItem>
            <UsefulInfoItem cmd="/push">Commits and pushes to the current branch.</UsefulInfoItem>
            <UsefulInfoItem cmd="/verify">
              Runs format, typecheck, test, and lint across the monorepo.
            </UsefulInfoItem>
            <UsefulInfoItem cmd="/pr">Commits, pushes, and opens a pull request.</UsefulInfoItem>
            <UsefulInfoItem cmd="/pr-fixup">
              Waits for CI and automated reviews, then fixes failures and addresses comments.
            </UsefulInfoItem>
          </div>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function UsefulInfoItem({ cmd, children }: { cmd: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <code className="font-mono text-[11px] text-foreground">{cmd}</code>
      <span>{children}</span>
    </div>
  );
}

function KindTabs({
  kind,
  onChange,
}: {
  kind: ImproveKind;
  onChange: (next: ImproveKind) => void;
}) {
  return (
    <Tabs value={kind} onValueChange={(v) => onChange(v as ImproveKind)}>
      <TabsList>
        <TabsTrigger value="bug" className="cursor-pointer">
          Bug fix
        </TabsTrigger>
        <TabsTrigger value="feature" className="cursor-pointer">
          Feature request
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

function BootstrapStatusSlot({
  bootstrap,
  captureLogs,
  setCaptureLogs,
}: {
  bootstrap: BootstrapState;
  captureLogs: boolean;
  setCaptureLogs: (v: boolean) => void;
}) {
  return (
    <div className="space-y-2">
      <BootstrapBanner bootstrap={bootstrap} />
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <label className="flex cursor-pointer items-center gap-2">
          <Checkbox checked={captureLogs} onCheckedChange={(v) => setCaptureLogs(v === true)} />
          Include recent backend &amp; browser logs as context for the agent
        </label>
        <TooltipProvider delayDuration={150}>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label="How log context works"
                className="cursor-help text-muted-foreground/70 hover:text-muted-foreground"
              >
                <IconInfoCircle className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="top" className="max-w-xs text-xs leading-relaxed">
              KanDev keeps a small in-memory ring buffer of the most recent backend logs and browser
              console events. When enabled, those logs are written to a temporary folder on your
              machine, and the file paths are appended to the task description so the agent can read
              them while investigating.
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    </div>
  );
}

function BootstrapBanner({ bootstrap }: { bootstrap: BootstrapState }) {
  if (bootstrap.kind === "loading" || bootstrap.kind === "idle") {
    return (
      <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        Preparing kandev repository in background. Fill in the details, submit when ready.
      </div>
    );
  }
  // The "blocked" kind is intercepted earlier in CreateModeView and routed to
  // BlockedDialog, so the form (and this banner) are not rendered for it. It
  // is still listed here to keep the union exhaustive for the type narrower.
  if (bootstrap.kind === "error" || bootstrap.kind === "blocked") {
    return (
      <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
        <IconAlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
        <span>{bootstrap.message}</span>
      </div>
    );
  }
  return <ContributorBanner data={bootstrap.data} />;
}

function BlockedDialog({
  open,
  onOpenChange,
  message,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  message: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconStethoscope className="h-5 w-5" />
            Improve Kandev
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            <IconAlertTriangle className="h-4 w-4 mt-0.5 shrink-0" />
            <span>{message}</span>
          </div>
          <div className="flex justify-end">
            <Button
              data-testid="improve-kandev-blocked-close"
              variant="outline"
              onClick={() => onOpenChange(false)}
              className="cursor-pointer"
            >
              Close
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ContributorBanner({ data }: { data: ImproveKandevBootstrapResponse }) {
  const { github_login: login, has_write_access: hasWrite } = data;
  return (
    <div className="flex items-start gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
      {hasWrite ? (
        <IconBrandGithub className="h-3.5 w-3.5 mt-0.5 shrink-0" />
      ) : (
        <IconGitFork className="h-3.5 w-3.5 mt-0.5 shrink-0" />
      )}
      <span>
        Contributing as{" "}
        {login ? (
          <code className="font-mono text-foreground">@{login}</code>
        ) : (
          <span>your GitHub account</span>
        )}
        .{" "}
        {hasWrite
          ? "You have write access to kdlbs/kandev, so the agent will push directly to a branch on the upstream repo."
          : "The agent will fork kdlbs/kandev to your account during the PR step and open a pull request from your fork."}
      </span>
    </div>
  );
}
