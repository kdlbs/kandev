"use client";

import { useCallback, useEffect, useId, useMemo, useState } from "react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { IconAlertTriangle } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { WatcherRepositoryFields } from "@/components/watcher-repository-fields";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useWorkflowSteps, stepPlaceholder } from "@/hooks/use-workflow-steps";
import { useWorkflows } from "@/hooks/use-workflows";
import { STEP_DEFAULT, STEP_DEFAULT_LABEL, resolveProfileId } from "@/lib/watcher-profile-default";
import type {
  CreateIssueWatchRequest,
  CreateReviewWatchRequest,
  UpdateIssueWatchRequest,
  UpdateReviewWatchRequest,
} from "@/lib/api/domains/gitlab-api";
import type { IssueWatch, ReviewWatch } from "@/lib/types/gitlab";
import {
  buildWatchPayload,
  makeWatchForm,
  watchFormFromWatch,
  type GitLabWatchForm,
  type GitLabWatchKind,
} from "./watch-form";

type Watch = ReviewWatch | IssueWatch;
type CreateRequest = CreateReviewWatchRequest | CreateIssueWatchRequest;
type UpdateRequest = UpdateReviewWatchRequest | UpdateIssueWatchRequest;

export type GitLabWatchDialogProps = {
  kind: GitLabWatchKind;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  watch: Watch | null;
  workspaceId: string;
  onCreate: (request: CreateRequest) => Promise<unknown>;
  onUpdate: (id: string, request: UpdateRequest) => Promise<unknown>;
};

type SelectItemShape = { id: string; label: string };

function SelectField(props: {
  label: string;
  description: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  items: SelectItemShape[];
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="min-w-0 space-y-1.5">
      <Label htmlFor={id}>{props.label}</Label>
      <p className="text-xs text-muted-foreground">{props.description}</p>
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
        <SelectTrigger id={id} className="w-full cursor-pointer">
          <SelectValue placeholder={props.placeholder} />
        </SelectTrigger>
        <SelectContent>
          {props.items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h3 className="border-b pb-2 text-sm font-medium">{children}</h3>;
}

function useDialogData(workspaceId: string, workflowId: string) {
  useSettingsData(true);
  useWorkflows(workspaceId, true);
  const workflows = useAppStore((state) => state.workflows.items).filter((item) => !item.hidden);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const executorProfiles = useMemo(
    () =>
      executors
        .filter((item) => item.type !== "local" && item.type !== "local_pc")
        .flatMap((item) => item.profiles ?? []),
    [executors],
  );
  const { steps, loading } = useWorkflowSteps(workflowId);
  return { workflows, agentProfiles, executorProfiles, steps, stepsLoading: loading };
}

function FilterFields({ kind, form, setForm }: FormFieldsProps) {
  return (
    <div className="space-y-4">
      <SectionTitle>Match</SectionTitle>
      <div className="space-y-1.5">
        <Label htmlFor={`${kind}-watch-projects`}>Project paths</Label>
        <p className="text-xs text-muted-foreground">
          Optional comma-separated namespace/project paths. Empty watches every accessible project.
        </p>
        <Input
          id={`${kind}-watch-projects`}
          value={form.projectPaths}
          onChange={(event) =>
            setForm((current) => ({ ...current, projectPaths: event.target.value }))
          }
          placeholder="group/api, group/web"
        />
      </div>
      {kind === "issue" && (
        <div className="space-y-1.5">
          <Label htmlFor="gitlab-watch-labels">Labels</Label>
          <p className="text-xs text-muted-foreground">
            Optional comma-separated GitLab labels that matching issues must include.
          </p>
          <Input
            id="gitlab-watch-labels"
            value={form.labels}
            onChange={(event) => setForm((current) => ({ ...current, labels: event.target.value }))}
            placeholder="bug, priority::high"
          />
        </div>
      )}
      <div className="space-y-1.5">
        <Label htmlFor={`${kind}-watch-query`}>GitLab query parameters</Label>
        <p className="text-xs text-muted-foreground">
          {kind === "review"
            ? "Leave empty to match merge requests requesting your review. Adding parameters explicitly replaces that reviewer constraint."
            : "Optional GitLab API query parameters, such as state=opened&milestone=Next."}
        </p>
        <Input
          id={`${kind}-watch-query`}
          value={form.customQuery}
          onChange={(event) =>
            setForm((current) => ({ ...current, customQuery: event.target.value }))
          }
          placeholder="state=opened"
          className="font-mono text-xs"
        />
      </div>
      {kind === "review" && (
        <SelectField
          label="Review scope"
          description="Choose whether to include only direct reviewer requests or the broader compatible scope."
          value={form.reviewScope}
          onChange={(value) =>
            setForm((current) => ({
              ...current,
              reviewScope: value as GitLabWatchForm["reviewScope"],
            }))
          }
          placeholder="Direct requests"
          items={[
            { id: "user", label: "Direct requests" },
            { id: "user_and_teams", label: "Direct and group-compatible requests" },
          ]}
        />
      )}
    </div>
  );
}

type FormFieldsProps = {
  kind: GitLabWatchKind;
  form: GitLabWatchForm;
  setForm: React.Dispatch<React.SetStateAction<GitLabWatchForm>>;
};

function AutomationFields({ kind, form, setForm }: FormFieldsProps) {
  const data = useDialogData(form.workspaceId, form.workflowId);
  return (
    <div className="space-y-4">
      <SectionTitle>Task automation</SectionTitle>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <SelectField
          label="Workflow"
          description="Workflow that receives new tasks."
          value={form.workflowId}
          onChange={(workflowId) =>
            setForm((current) => ({ ...current, workflowId, workflowStepId: "" }))
          }
          placeholder="Select workflow"
          items={data.workflows.map((item) => ({ id: item.id, label: item.name }))}
        />
        <SelectField
          label="Workflow step"
          description="Initial step for each new task."
          value={form.workflowStepId}
          onChange={(workflowStepId) => setForm((current) => ({ ...current, workflowStepId }))}
          placeholder={stepPlaceholder(form.workflowId, data.stepsLoading, data.steps.length)}
          items={data.steps.map((item) => ({ id: item.id, label: item.name }))}
          disabled={!form.workflowId || data.stepsLoading || data.steps.length === 0}
        />
      </div>
      <WatcherRepositoryFields
        workspaceId={form.workspaceId}
        repositoryId={form.repositoryId}
        baseBranch={form.baseBranch}
        onRepositoryChange={(repositoryId) =>
          setForm((current) => ({ ...current, repositoryId, baseBranch: "" }))
        }
        onBaseBranchChange={(baseBranch) => setForm((current) => ({ ...current, baseBranch }))}
      />
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <SelectField
          label="Agent profile"
          description="Optional; otherwise uses the workflow step default."
          value={form.agentProfileId || STEP_DEFAULT}
          onChange={(value) =>
            setForm((current) => ({ ...current, agentProfileId: resolveProfileId(value) }))
          }
          placeholder={STEP_DEFAULT_LABEL}
          items={[
            { id: STEP_DEFAULT, label: STEP_DEFAULT_LABEL },
            ...data.agentProfiles.map((item) => ({ id: item.id, label: item.label })),
          ]}
        />
        <SelectField
          label="Executor profile"
          description="Optional; otherwise uses the workflow step default."
          value={form.executorProfileId || STEP_DEFAULT}
          onChange={(value) =>
            setForm((current) => ({ ...current, executorProfileId: resolveProfileId(value) }))
          }
          placeholder={STEP_DEFAULT_LABEL}
          items={[
            { id: STEP_DEFAULT, label: STEP_DEFAULT_LABEL },
            ...data.executorProfiles.map((item) => ({ id: item.id, label: item.name })),
          ]}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${kind}-watch-prompt`}>Task prompt</Label>
        <p className="text-xs text-muted-foreground">
          Prompt sent to the selected agent profile for each new match.
        </p>
        <Textarea
          id={`${kind}-watch-prompt`}
          value={form.prompt}
          onChange={(event) => setForm((current) => ({ ...current, prompt: event.target.value }))}
          rows={5}
        />
      </div>
    </div>
  );
}

function ScheduleFields({ kind, form, setForm }: FormFieldsProps) {
  return (
    <div className="space-y-4">
      <SectionTitle>Polling and cleanup</SectionTitle>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={`${kind}-watch-interval`}>Poll interval (seconds)</Label>
          <p className="text-xs text-muted-foreground">Between 60 and 3600 seconds.</p>
          <Input
            id={`${kind}-watch-interval`}
            type="number"
            min={60}
            max={3600}
            value={form.pollIntervalSeconds}
            onChange={(event) =>
              setForm((current) => ({ ...current, pollIntervalSeconds: event.target.value }))
            }
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`${kind}-watch-inflight`}>Maximum in-flight tasks</Label>
          <p className="text-xs text-muted-foreground">
            Optional cap; polling resumes when active tasks fall below it.
          </p>
          <Input
            id={`${kind}-watch-inflight`}
            type="number"
            min={1}
            value={form.maxInflightTasks}
            onChange={(event) =>
              setForm((current) => ({ ...current, maxInflightTasks: event.target.value }))
            }
            placeholder="No limit"
          />
        </div>
      </div>
      <SelectField
        label="Cleanup policy"
        description="Controls task deletion when the GitLab item closes."
        value={form.cleanupPolicy}
        onChange={(value) =>
          setForm((current) => ({
            ...current,
            cleanupPolicy: value as GitLabWatchForm["cleanupPolicy"],
          }))
        }
        placeholder="Auto"
        items={[
          { id: "auto", label: "Auto; keep engaged tasks" },
          { id: "always", label: "Always delete" },
          { id: "never", label: "Never delete" },
        ]}
      />
    </div>
  );
}

export function GitLabWatchDialog({
  kind,
  open,
  onOpenChange,
  watch,
  workspaceId,
  onCreate,
  onUpdate,
}: GitLabWatchDialogProps) {
  const [form, setForm] = useState(() => makeWatchForm(kind, workspaceId));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    setForm(watch ? watchFormFromWatch(kind, watch) : makeWatchForm(kind, workspaceId));
    setError("");
  }, [kind, open, watch, workspaceId]);
  const payload = buildWatchPayload(kind as "review", form, Boolean(watch)) as CreateRequest | null;
  const save = useCallback(async () => {
    if (!payload) return;
    setSaving(true);
    setError("");
    try {
      if (watch) {
        const { workspace_id: _workspaceId, ...update } = payload;
        await onUpdate(watch.id, update);
      } else {
        await onCreate(payload);
      }
      onOpenChange(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "GitLab watch could not be saved");
    } finally {
      setSaving(false);
    }
  }, [onCreate, onOpenChange, onUpdate, payload, watch]);
  const noun = kind === "review" ? "review" : "issue";
  let saveLabel = "Create watch";
  if (watch) saveLabel = "Update watch";
  if (saving) saveLabel = "Saving...";
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-none overflow-y-auto sm:max-h-[90vh] sm:w-[min(900px,calc(100vw-2rem))]">
        <DialogHeader>
          <DialogTitle>
            {watch ? "Edit" : "Create"} GitLab {noun} watch
          </DialogTitle>
          <DialogDescription>
            Automatically create workspace tasks for newly matching GitLab{" "}
            {kind === "review" ? "merge requests" : "issues"}.
          </DialogDescription>
        </DialogHeader>
        {error && (
          <Alert variant="destructive">
            <IconAlertTriangle className="h-4 w-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <div className="space-y-6">
          <FilterFields kind={kind} form={form} setForm={setForm} />
          <AutomationFields kind={kind} form={form} setForm={setForm} />
          <ScheduleFields kind={kind} form={form} setForm={setForm} />
        </div>
        <DialogFooter className="gap-2 sm:gap-0">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="min-h-11 cursor-pointer sm:min-h-9"
          >
            Cancel
          </Button>
          <Button
            onClick={() => void save()}
            disabled={!payload || saving}
            className="min-h-11 cursor-pointer sm:min-h-9"
          >
            {saveLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
