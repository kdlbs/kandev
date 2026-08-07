/* eslint-disable max-lines, sonarjs/no-duplicate-string -- watcher settings intentionally co-locate both Azure watch forms and actions. */

"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlayerPlay, IconPlus, IconRefresh, IconTrash, IconX } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { SettingsSection } from "@/components/settings/settings-section";
import { formatDateTime } from "@/lib/i18n/formats";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAzureDevOpsWatches } from "@/hooks/domains/azure-devops/use-azure-devops-watches";
import type {
  AzureDevOpsCleanupPolicy,
  AzureDevOpsPullRequestWatch,
  AzureDevOpsPullRequestWatchInput,
  AzureDevOpsWorkItemWatch,
  AzureDevOpsWorkItemWatchInput,
} from "@/lib/types/azure-devops";

type Kind = "work-item" | "pull-request";

type Translate = (key: string, values?: Record<string, unknown>) => string;

/** Azure DevOps identity shorthand accepted by the creator/reviewer filters. */
const AZURE_ME_TOKEN = "@me";

/**
 * Catalog keys for the two watch kinds. `Kind` is a wire discriminant compared
 * with `===` and baked into `data-testid`s, so it is never rendered directly —
 * only the label resolves at render.
 */
const KIND_NOUN_KEYS: Record<Kind, string> = {
  "work-item": "azuredevops:workItemKindNoun",
  "pull-request": "azuredevops:pullRequestKindNoun",
};

/**
 * Sentence-cased cleanup-policy labels for the watch summary line. The map key
 * is the persisted `AzureDevOpsCleanupPolicy` value; the menu items in the
 * editor use the standalone (capitalized) forms instead.
 */
const CLEANUP_POLICY_KEYS: Record<string, string> = {
  auto: "azuredevops:cleanupAuto",
  always: "azuredevops:cleanupAlways",
  never: "azuredevops:cleanupNever",
};

/**
 * Sentence-cased labels for the watch card's status line. The map key is the
 * Azure DevOps pull-request status sent on the wire; `all` shares the label used
 * when no status is set, since both mean "not filtered by status". The editor's
 * menu items use the standalone (capitalized) forms instead.
 */
const WATCH_STATUS_KEYS: Record<string, string> = {
  active: "azuredevops:watchStatusActive",
  completed: "azuredevops:watchStatusCompleted",
  abandoned: "azuredevops:watchStatusAbandoned",
  all: "azuredevops:pullRequestAny",
};

/** An unrecognized status is echoed verbatim rather than blanked. */
function watchStatusLabel(t: Translate, status: string | undefined): string {
  if (!status) return t("azuredevops:pullRequestAny");
  const key = WATCH_STATUS_KEYS[status];
  return key ? t(key) : status;
}

function formatLastChecked(t: Translate, value?: string | null): string {
  if (!value) return t("azuredevops:notCheckedYet");
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return t("azuredevops:notCheckedYet");
  return t("azuredevops:checkedAt", { at: formatDateTime(date) });
}

// The two default prompts are PERSISTED on the watch and sent to the agent
// verbatim, and they carry `{{work_item.title}}` / `{{pull_request.title}}`
// placeholders the backend substitutes. They are deliberately NOT translated —
// see the GitLab and Sentry watch defaults for the same contract.
const emptyWorkItem: AzureDevOpsWorkItemWatchInput = {
  workflowId: "",
  workflowStepId: "",
  projectId: "",
  wiql: "",
  repositoryId: "",
  baseBranch: "",
  agentProfileId: "",
  executorProfileId: "",
  prompt: "Implement {{work_item.title}}",
  pollIntervalSeconds: 300,
  cleanupPolicy: "auto",
  maxInflightTasks: undefined,
};
const emptyPullRequest: AzureDevOpsPullRequestWatchInput = {
  workflowId: "",
  workflowStepId: "",
  projectId: "",
  azureRepositoryId: "",
  status: "active",
  creatorId: "",
  reviewerId: "",
  repositoryId: "",
  baseBranch: "",
  agentProfileId: "",
  executorProfileId: "",
  prompt: "Review {{pull_request.title}}",
  pollIntervalSeconds: 300,
  cleanupPolicy: "auto",
  maxInflightTasks: undefined,
};

function WatchActions({
  kind,
  watch,
  onEdit,
  onToggle,
  onTrigger,
  onReset,
  onDelete,
}: {
  kind: Kind;
  watch: AzureDevOpsWorkItemWatch | AzureDevOpsPullRequestWatch;
  onEdit: () => void;
  onToggle: () => void;
  onTrigger: () => void;
  onReset: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onEdit}
        data-testid={`azure-${kind}-watch-edit-${watch.id}`}
      >
        {t("azuredevops:edit")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onToggle}
        data-testid={`azure-${kind}-watch-toggle-${watch.id}`}
      >
        {watch.enabled ? t("azuredevops:disable") : t("azuredevops:enable")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onTrigger}
        data-testid={`azure-${kind}-watch-trigger-${watch.id}`}
      >
        <IconPlayerPlay className="h-4 w-4" /> {t("azuredevops:runNow")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onReset}
      >
        <IconRefresh className="h-4 w-4" /> {t("common:reset")}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="min-h-11 cursor-pointer text-destructive"
        onClick={onDelete}
      >
        <IconTrash className="h-4 w-4" /> {t("azuredevops:delete")}
      </Button>
    </div>
  );
}

function WatchCard({
  kind,
  watch,
  onEdit,
  onToggle,
  onTrigger,
  onReset,
  onDelete,
}: {
  kind: Kind;
  watch: AzureDevOpsWorkItemWatch | AzureDevOpsPullRequestWatch;
  onEdit: () => void;
  onToggle: () => void;
  onTrigger: () => void;
  onReset: () => void;
  onDelete: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Card data-testid={`azure-${kind}-watch-${watch.id}`}>
      <CardHeader className="space-y-2">
        <CardTitle className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span>
            {kind === "work-item"
              ? t("azuredevops:workItemQuery")
              : t("azuredevops:pullRequestFilter")}
          </span>
          <Badge variant={watch.enabled ? "default" : "secondary"}>
            {watch.enabled ? t("azuredevops:enabled") : t("azuredevops:disabled")}
          </Badge>
        </CardTitle>
        <div className="text-xs text-muted-foreground">
          {/* `projectId` is Azure DevOps data and `cleanupPolicy` a wire enum;
              both are interpolated rather than written into the message. */}
          {t("azuredevops:watchSummary", {
            projectId: watch.projectId,
            count: watch.pollIntervalSeconds,
            policy: t(CLEANUP_POLICY_KEYS[watch.cleanupPolicy] ?? CLEANUP_POLICY_KEYS.auto),
          })}
        </div>
        <div className="text-xs text-muted-foreground">
          {formatLastChecked(t, watch.lastPolledAt)}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {kind === "work-item" && (
          <pre className="max-h-20 overflow-auto rounded bg-muted/40 p-2 text-xs">
            {(watch as AzureDevOpsWorkItemWatch).wiql}
          </pre>
        )}
        {kind === "pull-request" && (
          <div className="text-xs text-muted-foreground">
            {t("azuredevops:statusLine", {
              status: watchStatusLabel(t, (watch as AzureDevOpsPullRequestWatch).status),
            })}
          </div>
        )}
        {watch.lastError && (
          <Alert variant="destructive">
            <AlertDescription>{watch.lastError}</AlertDescription>
          </Alert>
        )}
        <WatchActions
          kind={kind}
          watch={watch}
          onEdit={onEdit}
          onToggle={onToggle}
          onTrigger={onTrigger}
          onReset={onReset}
          onDelete={onDelete}
        />
      </CardContent>
    </Card>
  );
}

// eslint-disable-next-line max-lines-per-function, complexity -- the editor renders the shared responsive form for both watch kinds.
function WatchEditor({
  kind,
  open,
  onOpenChange,
  initial,
  editing,
  onSubmit,
}: {
  kind: Kind;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial?: AzureDevOpsWorkItemWatchInput | AzureDevOpsPullRequestWatchInput;
  editing: boolean;
  onSubmit: (
    input: AzureDevOpsWorkItemWatchInput | AzureDevOpsPullRequestWatchInput,
  ) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const [workItem, setWorkItem] = useState<AzureDevOpsWorkItemWatchInput>(
    kind === "work-item" && initial ? (initial as AzureDevOpsWorkItemWatchInput) : emptyWorkItem,
  );
  const [pullRequest, setPullRequest] = useState<AzureDevOpsPullRequestWatchInput>(
    kind === "pull-request" && initial
      ? (initial as AzureDevOpsPullRequestWatchInput)
      : emptyPullRequest,
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const current = kind === "work-item" ? workItem : pullRequest;
  const set = (key: string, value: string | number) => {
    if (kind === "work-item") setWorkItem((previous) => ({ ...previous, [key]: value }));
    else setPullRequest((previous) => ({ ...previous, [key]: value }));
  };
  const submit = async () => {
    if (
      !current.projectId ||
      !current.repositoryId ||
      !current.workflowId ||
      !current.workflowStepId ||
      !current.agentProfileId ||
      !current.executorProfileId ||
      (kind === "work-item" && !workItem.wiql)
    ) {
      setError(
        kind === "work-item"
          ? t("azuredevops:workItemWatchFieldsRequired")
          : t("azuredevops:pullRequestWatchFieldsRequired"),
      );
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSubmit(current);
      onOpenChange(false);
    } catch (cause) {
      setError(String(cause));
    } finally {
      setSaving(false);
    }
  };
  let submitLabel = editing ? t("azuredevops:updateWatch") : t("azuredevops:createWatch");
  if (saving) submitLabel = t("azuredevops:savingEllipsis");
  const kindNoun = t(KIND_NOUN_KEYS[kind]);
  const editorTitle = editing
    ? t("azuredevops:editWatchTitle", { kind: kindNoun })
    : t("azuredevops:newWatchTitle", { kind: kindNoun });
  const body = (
    <div className="min-h-0 space-y-4 overflow-y-auto p-4 pb-[env(safe-area-inset-bottom,0px)] sm:p-6">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>{t("azuredevops:azureProjectId")}</Label>
          <Input
            value={current.projectId}
            onChange={(event) => set("projectId", event.target.value)}
            data-testid={`azure-${kind}-watch-project`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:pollIntervalSeconds")}</Label>
          <Input
            type="number"
            min={60}
            value={current.pollIntervalSeconds}
            onChange={(event) => set("pollIntervalSeconds", Number(event.target.value))}
          />
        </div>
      </div>
      {kind === "work-item" ? (
        <div className="space-y-1.5">
          {/* WIQL is the Azure DevOps query language, not copy. */}
          <Label>WIQL</Label>
          <textarea
            className="min-h-28 w-full rounded-md border bg-background p-3 text-sm"
            value={workItem.wiql}
            onChange={(event) => set("wiql", event.target.value)}
            data-testid="azure-work-item-watch-wiql"
          />
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label>{t("azuredevops:azureRepositoryIdOptional")}</Label>
            <Input
              value={pullRequest.azureRepositoryId ?? ""}
              onChange={(event) => set("azureRepositoryId", event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t("azuredevops:status")}</Label>
            <Select
              value={pullRequest.status || "active"}
              onValueChange={(value) => set("status", value)}
            >
              <SelectTrigger className="min-h-11">
                <SelectValue />
              </SelectTrigger>
              {/* `value` is the Azure DevOps pull-request status sent on the
                  wire; only the item label is copy. */}
              <SelectContent>
                <SelectItem value="active">{t("azuredevops:statusActive")}</SelectItem>
                <SelectItem value="completed">{t("azuredevops:statusCompleted")}</SelectItem>
                <SelectItem value="abandoned">{t("azuredevops:statusAbandoned")}</SelectItem>
                <SelectItem value="all">{t("azuredevops:statusAny")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label>{t("azuredevops:creatorId")}</Label>
            <Input
              value={pullRequest.creatorId ?? ""}
              onChange={(event) => set("creatorId", event.target.value)}
              placeholder={t("azuredevops:identityPlaceholder", { token: AZURE_ME_TOKEN })}
            />
          </div>
          <div className="space-y-1.5">
            <Label>{t("azuredevops:reviewerId")}</Label>
            <Input
              value={pullRequest.reviewerId ?? ""}
              onChange={(event) => set("reviewerId", event.target.value)}
              placeholder={t("azuredevops:identityPlaceholder", { token: AZURE_ME_TOKEN })}
            />
          </div>
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>{t("azuredevops:kandevRepositoryId")}</Label>
          <Input
            value={current.repositoryId}
            onChange={(event) => set("repositoryId", event.target.value)}
            data-testid={`azure-${kind}-watch-repository`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:baseBranch")}</Label>
          <Input
            value={current.baseBranch}
            onChange={(event) => set("baseBranch", event.target.value)}
            placeholder="main"
            data-testid={`azure-${kind}-watch-branch`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:workflowId")}</Label>
          <Input
            value={current.workflowId}
            onChange={(event) => set("workflowId", event.target.value)}
            data-testid={`azure-${kind}-watch-workflow`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:workflowStepId")}</Label>
          <Input
            value={current.workflowStepId}
            onChange={(event) => set("workflowStepId", event.target.value)}
            data-testid={`azure-${kind}-watch-step`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:agentProfileId")}</Label>
          <Input
            value={current.agentProfileId}
            onChange={(event) => set("agentProfileId", event.target.value)}
            data-testid={`azure-${kind}-watch-agent`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:executorProfileId")}</Label>
          <Input
            value={current.executorProfileId}
            onChange={(event) => set("executorProfileId", event.target.value)}
            data-testid={`azure-${kind}-watch-executor`}
          />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label>{t("azuredevops:prompt")}</Label>
        <textarea
          className="min-h-24 w-full rounded-md border bg-background p-3 text-sm"
          value={current.prompt}
          onChange={(event) => set("prompt", event.target.value)}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>{t("azuredevops:cleanupPolicy")}</Label>
          <Select
            value={current.cleanupPolicy}
            onValueChange={(value: AzureDevOpsCleanupPolicy) => set("cleanupPolicy", value)}
          >
            <SelectTrigger className="min-h-11">
              <SelectValue />
            </SelectTrigger>
            {/* `value` is the persisted `AzureDevOpsCleanupPolicy`; only the
                item label is copy. */}
            <SelectContent>
              <SelectItem value="auto">{t("azuredevops:cleanupPolicyAuto")}</SelectItem>
              <SelectItem value="always">{t("azuredevops:cleanupPolicyAlways")}</SelectItem>
              <SelectItem value="never">{t("azuredevops:cleanupPolicyNever")}</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label>{t("azuredevops:maxInflightTasks")}</Label>
          <Input
            type="number"
            min={0}
            value={current.maxInflightTasks ?? 0}
            onChange={(event) => set("maxInflightTasks", Number(event.target.value))}
          />
        </div>
      </div>
      <Button
        type="button"
        className="min-h-11 w-full cursor-pointer"
        onClick={() => void submit()}
        disabled={saving}
      >
        {submitLabel}
      </Button>
    </div>
  );
  if (isMobile)
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        <DrawerContent className="h-[100dvh] max-h-[100dvh] overflow-hidden">
          <DrawerHeader className="flex items-center justify-between gap-3">
            <DrawerTitle>{editorTitle}</DrawerTitle>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="min-h-11 min-w-11 cursor-pointer"
              aria-label={t("azuredevops:closeWatchEditor")}
              data-testid="azure-watch-editor-close"
              onClick={() => onOpenChange(false)}
            >
              <IconX className="h-4 w-4" />
            </Button>
          </DrawerHeader>
          {body}
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-hidden p-0">
        <DialogHeader className="flex flex-row items-center justify-between gap-3 px-6 pt-6">
          <DialogTitle>{editorTitle}</DialogTitle>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="min-h-11 min-w-11 cursor-pointer"
            aria-label={t("azuredevops:closeWatchEditor")}
            data-testid="azure-watch-editor-close"
            onClick={() => onOpenChange(false)}
          >
            <IconX className="h-4 w-4" />
          </Button>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  );
}

// eslint-disable-next-line max-lines-per-function -- settings coordinates watch lists, editor state, and destructive confirmations.
export function AzureDevOpsWatchSettings({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const watches = useAzureDevOpsWatches(workspaceId);
  const [editor, setEditor] = useState<{
    kind: Kind;
    id?: string;
    initial?: AzureDevOpsWorkItemWatchInput | AzureDevOpsPullRequestWatchInput;
  } | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const run = async (kind: Kind, id: string) => {
    try {
      const result = await watches.trigger(kind, id);
      setMessage(t("azuredevops:newMatches", { count: result.matched }));
    } catch (error) {
      setMessage(String(error));
    }
  };
  const reset = async (kind: Kind, id: string, policy: string) => {
    try {
      const preview = await watches.previewReset(kind, id);
      const confirmed = confirm(
        t("azuredevops:resetWatchConfirm", {
          policy: t(CLEANUP_POLICY_KEYS[policy] ?? CLEANUP_POLICY_KEYS.auto),
          count: preview.taskCount,
        }),
      );
      if (!confirmed) return;
      await watches.reset(kind, id);
      setMessage(t("azuredevops:watchReset"));
    } catch (error) {
      setMessage(String(error));
    }
  };
  const toggle = async (kind: Kind, id: string, enabled: boolean) => {
    try {
      if (kind === "work-item") await watches.updateWorkItem(id, { enabled });
      else await watches.updatePullRequest(id, { enabled });
      setMessage(enabled ? t("azuredevops:watchEnabled") : t("azuredevops:watchDisabled"));
    } catch (error) {
      setMessage(String(error));
    }
  };
  const remove = async (kind: Kind, id: string) => {
    if (!confirm(t("azuredevops:deleteWatchConfirm"))) return;
    try {
      await watches.remove(kind, id);
      setMessage(t("azuredevops:watchDeleted"));
    } catch (error) {
      setMessage(String(error));
    }
  };
  return (
    <div className="space-y-6" data-testid="azure-devops-watch-settings">
      {message && (
        <Alert>
          <AlertDescription>{message}</AlertDescription>
        </Alert>
      )}
      {watches.error && (
        <Alert variant="destructive">
          <AlertDescription>{watches.error}</AlertDescription>
        </Alert>
      )}
      {watches.loading && (
        <p className="text-sm text-muted-foreground">{t("azuredevops:loadingWatchers")}</p>
      )}
      <SettingsSection
        title={t("azuredevops:pullRequestWatches")}
        description={t("azuredevops:pullRequestWatchesDescription")}
        action={
          <Button
            type="button"
            className="min-h-11 w-full cursor-pointer sm:w-auto"
            onClick={() => setEditor({ kind: "pull-request" })}
            data-testid="azure-add-pull-request-watch"
          >
            <IconPlus className="h-4 w-4" /> {t("azuredevops:addPrWatch")}
          </Button>
        }
      >
        {!watches.loading && watches.pullRequests.length === 0 && (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">
              {t("azuredevops:noPullRequestWatches")}
            </CardContent>
          </Card>
        )}
        <div className="grid gap-4 xl:grid-cols-2">
          {watches.pullRequests.map((watch) => (
            <WatchCard
              key={watch.id}
              kind="pull-request"
              watch={watch}
              onEdit={() => setEditor({ kind: "pull-request", id: watch.id, initial: watch })}
              onToggle={() => void toggle("pull-request", watch.id, !watch.enabled)}
              onTrigger={() => void run("pull-request", watch.id)}
              onReset={() => void reset("pull-request", watch.id, watch.cleanupPolicy)}
              onDelete={() => void remove("pull-request", watch.id)}
            />
          ))}
        </div>
      </SettingsSection>
      <SettingsSection
        title={t("azuredevops:workItemWatches")}
        description={t("azuredevops:workItemWatchesDescription")}
        action={
          <Button
            type="button"
            className="min-h-11 w-full cursor-pointer sm:w-auto"
            onClick={() => setEditor({ kind: "work-item" })}
            data-testid="azure-add-work-item-watch"
          >
            <IconPlus className="h-4 w-4" /> {t("azuredevops:addWorkItemWatch")}
          </Button>
        }
      >
        {!watches.loading && watches.workItems.length === 0 && (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">
              {t("azuredevops:noWorkItemWatches")}
            </CardContent>
          </Card>
        )}
        <div className="grid gap-4 xl:grid-cols-2">
          {watches.workItems.map((watch) => (
            <WatchCard
              key={watch.id}
              kind="work-item"
              watch={watch}
              onEdit={() => setEditor({ kind: "work-item", id: watch.id, initial: watch })}
              onToggle={() => void toggle("work-item", watch.id, !watch.enabled)}
              onTrigger={() => void run("work-item", watch.id)}
              onReset={() => void reset("work-item", watch.id, watch.cleanupPolicy)}
              onDelete={() => void remove("work-item", watch.id)}
            />
          ))}
        </div>
      </SettingsSection>
      {editor && (
        <WatchEditor
          key={`${editor.kind}:${editor.id ?? "new"}`}
          kind={editor.kind}
          open
          editing={Boolean(editor.id)}
          initial={editor.initial}
          onOpenChange={(open) => !open && setEditor(null)}
          onSubmit={(input) => {
            if (editor.kind === "work-item") {
              const value = input as AzureDevOpsWorkItemWatchInput;
              return editor.id
                ? watches.updateWorkItem(editor.id, value)
                : watches.createWorkItem(value);
            }
            const value = input as AzureDevOpsPullRequestWatchInput;
            return editor.id
              ? watches.updatePullRequest(editor.id, value)
              : watches.createPullRequest(value);
          }}
        />
      )}
    </div>
  );
}
