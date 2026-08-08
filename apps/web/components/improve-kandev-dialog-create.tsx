"use client";

import { Fragment, useState } from "react";
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
} from "@tabler/icons-react";

import { TaskCreateDialog } from "@/components/task-create-dialog";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";
import { cn } from "@/lib/utils";
import type { Task, WorkflowStep } from "@/lib/types/http";
import {
  contributorAccessMessage,
  getImproveKandevForkBlockedReason,
  getImproveKandevStepDescription,
  resolveImproveKandevWorkflow,
  type ImproveKandevKind,
} from "./improve-kandev-dialog-model";
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

export type BootstrapState =
  | { kind: "idle" }
  | { kind: "loading" }
  | {
      kind: "ready";
      data: ImproveKandevBootstrapResponse;
      steps: WorkflowStep[];
      issueSteps: WorkflowStep[];
    }
  | { kind: "error"; message: string };

type ImproveKind = ImproveKandevKind;

// Surfaced in the dialog's submit-button tooltip while the bootstrap probe is
// in flight, so the disabled state has an explanation the user can act on
// instead of the usual missing-field reasons.
function bootstrapBlockedReason(t: TFunction, bootstrap: BootstrapState): string | null {
  if (bootstrap.kind === "loading" || bootstrap.kind === "idle") {
    return t("common:preparingKandevRepository");
  }
  if (bootstrap.kind === "error") {
    return t("common:bootstrapFailedCloseAndReopen");
  }
  return null;
}

// Keys, not resolved copy: a `t()` at module scope would freeze at the boot
// locale and the pseudo-locale could not see it.
const PLACEHOLDER_KEYS: Record<ImproveKind, string> = {
  bug: "common:improveKandevBugPlaceholder",
  feature: "common:improveKandevFeaturePlaceholder",
  issue: "common:improveKandevIssuePlaceholder",
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
  externalBlockedReason?: string | null;
};

export function CreateModeView(props: CreateModeViewProps) {
  const { t } = useTranslation();
  const { bootstrap, captureLogs, setCaptureLogs } = props;
  const [kind, setKind] = useState<ImproveKind>("bug");
  const ready = bootstrap.kind === "ready" ? bootstrap : null;
  const workflow = resolveImproveKandevWorkflow(ready, kind);
  const forkBlockedReason = ready
    ? getImproveKandevForkBlockedReason(kind, ready.data.fork_status, ready.data.fork_message)
    : null;
  // Tasks land in the dedicated workspace the bootstrap returned, not the
  // user's active one. While bootstrap is in flight the active workspace is
  // only a fallback — submit stays blocked until `ready`.
  const effectiveWorkspaceId = ready ? ready.data.workspace_id : props.workspaceId;

  const handleKindChange = (next: ImproveKind) => {
    setKind(next);
    setCaptureLogs(next === "bug");
  };

  return (
    <TaskCreateDialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      mode="create"
      workspaceId={effectiveWorkspaceId}
      workflowId={workflow.workflowId}
      defaultStepId={workflow.startStep?.id ?? null}
      steps={workflow.steps.map((s) => ({ id: s.id, title: s.name, events: s.events }))}
      initialValues={{
        title: "",
        repositoryId: ready?.data.repository_id ?? "",
        branch: ready?.data.branch ?? "",
      }}
      onSuccess={props.onTaskCreated}
      lockedFields={{ repository: true, branch: true, workflow: true }}
      submitBlockedReason={
        props.externalBlockedReason ?? forkBlockedReason ?? bootstrapBlockedReason(t, bootstrap)
      }
      descriptionPlaceholder={t(PLACEHOLDER_KEYS[kind])}
      aboveDescriptionSlot={<KindTabs kind={kind} onChange={handleKindChange} />}
      extraFormSlot={
        <BootstrapStatusSlot
          bootstrap={bootstrap}
          kind={kind}
          captureLogs={captureLogs}
          setCaptureLogs={setCaptureLogs}
        />
      }
      bottomSlot={
        ready && (
          <div className="space-y-2">
            <CreateModeBottomSlot kind={kind} steps={workflow.steps} />
          </div>
        )
      }
      transformDescriptionBeforeSubmit={props.transformDescription}
    />
  );
}

function CreateModeBottomSlot({ kind, steps }: { kind: ImproveKind; steps: WorkflowStep[] }) {
  return (
    <>
      <WorkflowStepsPreview steps={steps} />
      {kind !== "issue" && <UsefulInfoCollapsible />}
    </>
  );
}

function WorkflowStepsPreview({ steps }: { steps: WorkflowStep[] }) {
  const { t } = useTranslation();
  if (steps.length === 0) return null;
  const ordered = [...steps].sort((a, b) => a.position - b.position);
  return (
    <div className="rounded-md border border-border bg-muted/30 px-3 py-2">
      <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wide text-muted-foreground/80">
        {t("common:workflow")}
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
                  {getImproveKandevStepDescription(s)}
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
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="group flex w-full cursor-pointer items-center justify-between rounded-md border border-border bg-muted/30 px-3 py-2 text-left text-xs hover:bg-muted/50"
        >
          <span className="font-medium text-muted-foreground">
            {t("common:usefulCommandsAgentSkills")}
          </span>
          <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground transition-transform group-data-[state=open]:rotate-180" />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 space-y-3 rounded-md border border-border bg-muted/20 px-3 py-2 text-xs leading-relaxed text-muted-foreground">
          <p>
            <Trans i18nKey="common:shellCommandPlusSlashCommandSkills">
              Shell command you can run in the secondary instance, plus slash-command{" "}
              <em>skills</em> you can ask the agent to run during the workflow.
            </Trans>
          </p>
          <div className="space-y-2">
            <p className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground/70">
              {t("common:shell")}
            </p>
            <UsefulInfoItem cmd="make install && make dev">
              {t("common:bootsASecondaryKandevDevInstance")}
            </UsefulInfoItem>
          </div>
          <div className="space-y-2">
            <p className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground/70">
              {t("common:agentSkillsAskTheAgentTo")}
            </p>
            <UsefulInfoItem cmd="/commit">
              {t("common:stagesAndCommitsChangesUsingConventional")}
            </UsefulInfoItem>
            <UsefulInfoItem cmd="/push">{t("common:commitsAndPushesToTheCurrent")}</UsefulInfoItem>
            <UsefulInfoItem cmd="/verify">
              {t("common:runsFormatTypecheckTestAndLint")}
            </UsefulInfoItem>
            <UsefulInfoItem cmd="/pr">{t("common:commitsPushesAndOpensAPull")}</UsefulInfoItem>
            <UsefulInfoItem cmd="/pr-fixup">
              {t("common:waitsForCiAndAutomatedReviews")}
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
  const { t } = useTranslation();
  return (
    <Tabs value={kind} onValueChange={(v) => onChange(v as ImproveKind)}>
      <TabsList>
        <TabsTrigger value="bug" className="cursor-pointer">
          {t("common:bugFix")}
        </TabsTrigger>
        <TabsTrigger value="feature" className="cursor-pointer">
          {t("common:featureRequest")}
        </TabsTrigger>
        <TabsTrigger value="issue" className="cursor-pointer">
          {t("common:openIssue")}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );
}

function BootstrapStatusSlot({
  bootstrap,
  kind,
  captureLogs,
  setCaptureLogs,
}: {
  bootstrap: BootstrapState;
  kind: ImproveKind;
  captureLogs: boolean;
  setCaptureLogs: (v: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <BootstrapBanner bootstrap={bootstrap} kind={kind} />
      {kind === "issue" && (
        <div
          className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-muted-foreground"
          data-testid="improve-kandev-issue-only-notice"
        >
          {t("common:thisWorkflowWillOnlyCreateA")}
        </div>
      )}
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <label className="flex cursor-pointer items-center gap-2">
          <Checkbox checked={captureLogs} onCheckedChange={(v) => setCaptureLogs(v === true)} />
          {t("common:includeRecentBackendAndBrowserLogs")}
        </label>
        <TooltipProvider delayDuration={150}>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={t("common:howLogContextWorks")}
                className="cursor-help text-muted-foreground/70 hover:text-muted-foreground"
              >
                <IconInfoCircle className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="top" className="max-w-xs text-xs leading-relaxed">
              {t("common:kandevKeepsASmallInMemory")}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    </div>
  );
}

function BootstrapBanner({ bootstrap, kind }: { bootstrap: BootstrapState; kind: ImproveKind }) {
  const { t } = useTranslation();
  if (bootstrap.kind === "loading" || bootstrap.kind === "idle") {
    return (
      <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" />
        {t("common:preparingKandevRepositoryInBackground")}
      </div>
    );
  }
  if (bootstrap.kind === "error") {
    return (
      <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
        <IconAlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
        <span>{bootstrap.message}</span>
      </div>
    );
  }
  return <ContributorBanner data={bootstrap.data} kind={kind} />;
}

function ContributorBanner({
  data,
  kind,
}: {
  data: ImproveKandevBootstrapResponse;
  kind: ImproveKind;
}) {
  const { t } = useTranslation();
  const { github_login: login, has_write_access: hasWrite } = data;
  if (data.fork_status === "blocked_emu" && kind !== "issue") {
    return (
      <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
        <IconAlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
        <span>{data.fork_message ?? t("common:thisGithubAccountCannotForkKdlbs")}</span>
      </div>
    );
  }
  const accessMessage = contributorAccessMessage(kind, hasWrite);
  return (
    <div className="flex items-start gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
      {hasWrite ? (
        <IconBrandGithub className="h-3.5 w-3.5 mt-0.5 shrink-0" />
      ) : (
        <IconGitFork className="h-3.5 w-3.5 mt-0.5 shrink-0" />
      )}
      {/* One complete sentence per branch. Splitting this into a "Contributing
          as" stem plus a tail would freeze the word order in JSX and leave the
          tail untranslatable. */}
      <span>
        {login ? (
          <Trans i18nKey="common:contributingAsLogin" values={{ login, access: accessMessage }}>
            Contributing as <code className="font-mono text-foreground">@{login}</code>.{" "}
            {accessMessage}
          </Trans>
        ) : (
          t("common:contributingAsYourGithubAccount", { access: accessMessage })
        )}
      </span>
    </div>
  );
}
