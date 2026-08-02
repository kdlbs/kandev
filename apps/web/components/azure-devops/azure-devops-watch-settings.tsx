/* eslint-disable max-lines, sonarjs/no-duplicate-string -- watcher settings intentionally co-locate both Azure watch forms and actions. */

"use client";

import { useState } from "react";
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

function formatLastChecked(value?: string | null): string {
  if (!value) return "Not checked yet";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "Not checked yet" : `checked ${date.toLocaleString()}`;
}

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
        Edit
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onToggle}
        data-testid={`azure-${kind}-watch-toggle-${watch.id}`}
      >
        {watch.enabled ? "Disable" : "Enable"}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onTrigger}
        data-testid={`azure-${kind}-watch-trigger-${watch.id}`}
      >
        <IconPlayerPlay className="h-4 w-4" /> Run now
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="min-h-11 cursor-pointer"
        onClick={onReset}
      >
        <IconRefresh className="h-4 w-4" /> Reset
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="min-h-11 cursor-pointer text-destructive"
        onClick={onDelete}
      >
        <IconTrash className="h-4 w-4" /> Delete
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
  return (
    <Card data-testid={`azure-${kind}-watch-${watch.id}`}>
      <CardHeader className="space-y-2">
        <CardTitle className="flex flex-wrap items-center justify-between gap-2 text-sm">
          <span>{kind === "work-item" ? "Work-item query" : "Pull-request filter"}</span>
          <Badge variant={watch.enabled ? "default" : "secondary"}>
            {watch.enabled ? "Enabled" : "Disabled"}
          </Badge>
        </CardTitle>
        <div className="text-xs text-muted-foreground">
          {watch.projectId} · every {watch.pollIntervalSeconds}s · cleanup {watch.cleanupPolicy}
        </div>
        <div className="text-xs text-muted-foreground">{formatLastChecked(watch.lastPolledAt)}</div>
      </CardHeader>
      <CardContent className="space-y-3">
        {kind === "work-item" && (
          <pre className="max-h-20 overflow-auto rounded bg-muted/40 p-2 text-xs">
            {(watch as AzureDevOpsWorkItemWatch).wiql}
          </pre>
        )}
        {kind === "pull-request" && (
          <div className="text-xs text-muted-foreground">
            Status: {(watch as AzureDevOpsPullRequestWatch).status || "any"}
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
          ? "Project, WIQL, repository, workflow, step, agent, and executor are required."
          : "Project, repository, workflow, step, agent, and executor are required.",
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
  let submitLabel = editing ? "Update watch" : "Create watch";
  if (saving) submitLabel = "Saving…";
  const watchLabel = kind === "work-item" ? "work-item" : "pull-request";
  const editorTitle = `${editing ? "Edit" : "New"} ${watchLabel} watch`;
  const body = (
    <div className="min-h-0 space-y-4 overflow-y-auto p-4 pb-[env(safe-area-inset-bottom,0px)] sm:p-6">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>Azure project ID</Label>
          <Input
            value={current.projectId}
            onChange={(event) => set("projectId", event.target.value)}
            data-testid={`azure-${kind}-watch-project`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Poll interval (seconds)</Label>
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
            <Label>Azure repository ID (optional)</Label>
            <Input
              value={pullRequest.azureRepositoryId ?? ""}
              onChange={(event) => set("azureRepositoryId", event.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label>Status</Label>
            <Select
              value={pullRequest.status || "active"}
              onValueChange={(value) => set("status", value)}
            >
              <SelectTrigger className="min-h-11">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="completed">Completed</SelectItem>
                <SelectItem value="abandoned">Abandoned</SelectItem>
                <SelectItem value="all">Any status</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label>Creator ID</Label>
            <Input
              value={pullRequest.creatorId ?? ""}
              onChange={(event) => set("creatorId", event.target.value)}
              placeholder="@me or identity"
            />
          </div>
          <div className="space-y-1.5">
            <Label>Reviewer ID</Label>
            <Input
              value={pullRequest.reviewerId ?? ""}
              onChange={(event) => set("reviewerId", event.target.value)}
              placeholder="@me or identity"
            />
          </div>
        </div>
      )}
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>Kandev repository ID</Label>
          <Input
            value={current.repositoryId}
            onChange={(event) => set("repositoryId", event.target.value)}
            data-testid={`azure-${kind}-watch-repository`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Base branch</Label>
          <Input
            value={current.baseBranch}
            onChange={(event) => set("baseBranch", event.target.value)}
            placeholder="main"
            data-testid={`azure-${kind}-watch-branch`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Workflow ID</Label>
          <Input
            value={current.workflowId}
            onChange={(event) => set("workflowId", event.target.value)}
            data-testid={`azure-${kind}-watch-workflow`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Workflow step ID</Label>
          <Input
            value={current.workflowStepId}
            onChange={(event) => set("workflowStepId", event.target.value)}
            data-testid={`azure-${kind}-watch-step`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Agent profile ID</Label>
          <Input
            value={current.agentProfileId}
            onChange={(event) => set("agentProfileId", event.target.value)}
            data-testid={`azure-${kind}-watch-agent`}
          />
        </div>
        <div className="space-y-1.5">
          <Label>Executor profile ID</Label>
          <Input
            value={current.executorProfileId}
            onChange={(event) => set("executorProfileId", event.target.value)}
            data-testid={`azure-${kind}-watch-executor`}
          />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label>Prompt</Label>
        <textarea
          className="min-h-24 w-full rounded-md border bg-background p-3 text-sm"
          value={current.prompt}
          onChange={(event) => set("prompt", event.target.value)}
        />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>Cleanup policy</Label>
          <Select
            value={current.cleanupPolicy}
            onValueChange={(value: AzureDevOpsCleanupPolicy) =>
              set(kind === "work-item" ? "cleanupPolicy" : "cleanupPolicy", value)
            }
          >
            <SelectTrigger className="min-h-11">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="auto">Auto</SelectItem>
              <SelectItem value="always">Always</SelectItem>
              <SelectItem value="never">Never</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label>Max in-flight tasks (0 = unlimited)</Label>
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
              aria-label="Close watch editor"
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
            aria-label="Close watch editor"
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
      setMessage(`${result.matched} new match${result.matched === 1 ? "" : "es"}.`);
    } catch (error) {
      setMessage(String(error));
    }
  };
  const reset = async (kind: Kind, id: string, policy: string) => {
    try {
      const preview = await watches.previewReset(kind, id);
      if (!confirm(`Reset this ${policy} watch and remove ${preview.taskCount} task(s)?`)) return;
      await watches.reset(kind, id);
      setMessage("Watch reset.");
    } catch (error) {
      setMessage(String(error));
    }
  };
  const toggle = async (kind: Kind, id: string, enabled: boolean) => {
    try {
      if (kind === "work-item") await watches.updateWorkItem(id, { enabled });
      else await watches.updatePullRequest(id, { enabled });
      setMessage(enabled ? "Watch enabled." : "Watch disabled.");
    } catch (error) {
      setMessage(String(error));
    }
  };
  const remove = async (kind: Kind, id: string) => {
    if (!confirm("Delete this watch?")) return;
    try {
      await watches.remove(kind, id);
      setMessage("Watch deleted.");
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
      {watches.loading && <p className="text-sm text-muted-foreground">Loading watchers…</p>}
      <SettingsSection
        title="Pull-request watches"
        description="Create tasks automatically from matching Azure DevOps pull requests."
        action={
          <Button
            type="button"
            className="min-h-11 w-full cursor-pointer sm:w-auto"
            onClick={() => setEditor({ kind: "pull-request" })}
            data-testid="azure-add-pull-request-watch"
          >
            <IconPlus className="h-4 w-4" /> Add PR watch
          </Button>
        }
      >
        {!watches.loading && watches.pullRequests.length === 0 && (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">
              No pull-request watches yet.
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
        title="Work-item watches"
        description="Create tasks automatically from matching Azure DevOps work-item queries."
        action={
          <Button
            type="button"
            className="min-h-11 w-full cursor-pointer sm:w-auto"
            onClick={() => setEditor({ kind: "work-item" })}
            data-testid="azure-add-work-item-watch"
          >
            <IconPlus className="h-4 w-4" /> Add work-item watch
          </Button>
        }
      >
        {!watches.loading && watches.workItems.length === 0 && (
          <Card>
            <CardContent className="p-6 text-sm text-muted-foreground">
              No work-item watches yet.
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
