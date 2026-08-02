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
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

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
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <SectionTitle>{t("gitlab:match")}</SectionTitle>
      <div className="space-y-1.5">
        <Label htmlFor={`${kind}-watch-projects`}>{t("gitlab:projectPaths")}</Label>
        <p className="text-xs text-muted-foreground">
          {t("gitlab:optionalCommaSeparatedNamespaceProjectPaths")}
        </p>
        <Input
          id={`${kind}-watch-projects`}
          value={form.projectPaths}
          onChange={(event) =>
            setForm((current) => ({ ...current, projectPaths: event.target.value }))
          }
          // Sample namespace/project paths, i.e. the shape of the GitLab data the
          // field accepts. Not copy — a translated "group" would stop being a
          // usable example.
          placeholder="group/api, group/web"
        />
      </div>
      {kind === "issue" && (
        <div className="space-y-1.5">
          <Label htmlFor="gitlab-watch-labels">{t("gitlab:labels")}</Label>
          <p className="text-xs text-muted-foreground">
            {t("gitlab:optionalCommaSeparatedGitlabLabelsThat")}
          </p>
          <Input
            id="gitlab-watch-labels"
            value={form.labels}
            onChange={(event) => setForm((current) => ({ ...current, labels: event.target.value }))}
            // Sample GitLab label names — user data, like the labels the user
            // will actually type here.
            placeholder="bug, priority::high"
          />
        </div>
      )}
      <div className="space-y-1.5">
        <Label htmlFor={`${kind}-watch-query`}>{t("gitlab:gitlabQueryParameters")}</Label>
        <p className="text-xs text-muted-foreground">
          {kind === "review"
            ? t("gitlab:leaveEmptyToMatchMergeRequests")
            : // The example is a literal GitLab API query string, so it is
              // interpolated rather than written into the catalog — a translated
              // (or pseudo-transliterated) copy of it would not parse.
              t("gitlab:optionalGitlabApiQueryParametersSuch", {
                example: "state=opened&milestone=Next",
              })}
        </p>
        <Input
          id={`${kind}-watch-query`}
          value={form.customQuery}
          onChange={(event) =>
            setForm((current) => ({ ...current, customQuery: event.target.value }))
          }
          // A literal GitLab API query parameter. It is submitted to GitLab
          // verbatim, so it is protocol, not copy.
          placeholder="state=opened"
          className="font-mono text-xs"
        />
      </div>
      {kind === "review" && (
        <SelectField
          label={t("gitlab:reviewScope")}
          description={t("gitlab:chooseWhetherToIncludeOnlyDirect")}
          value={form.reviewScope}
          onChange={(value) =>
            setForm((current) => ({
              ...current,
              reviewScope: value as GitLabWatchForm["reviewScope"],
            }))
          }
          placeholder={t("gitlab:directRequests")}
          items={[
            { id: "user", label: t("gitlab:directRequests") },
            { id: "user_and_teams", label: t("gitlab:directAndGroupCompatibleRequests") },
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
  const { t } = useTranslation();
  const data = useDialogData(form.workspaceId, form.workflowId);
  return (
    <div className="space-y-4">
      <SectionTitle>{t("gitlab:taskAutomation")}</SectionTitle>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <SelectField
          label={t("gitlab:workflow")}
          description={t("gitlab:workflowThatReceivesNewTasks")}
          value={form.workflowId}
          onChange={(workflowId) =>
            setForm((current) => ({ ...current, workflowId, workflowStepId: "" }))
          }
          placeholder={t("gitlab:selectWorkflow")}
          items={data.workflows.map((item) => ({ id: item.id, label: item.name }))}
        />
        <SelectField
          label={t("gitlab:workflowStep")}
          description={t("gitlab:initialStepForEachNewTask")}
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
          label={t("gitlab:agentProfile")}
          description={t("gitlab:optionalOtherwiseUsesTheWorkflowStep")}
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
          label={t("gitlab:executorProfile")}
          description={t("gitlab:optionalOtherwiseUsesTheWorkflowStep")}
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
        <Label htmlFor={`${kind}-watch-prompt`}>{t("gitlab:taskPrompt")}</Label>
        <p className="text-xs text-muted-foreground">{t("gitlab:promptSentToTheSelectedAgent")}</p>
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
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <SectionTitle>{t("gitlab:pollingAndCleanup")}</SectionTitle>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={`${kind}-watch-interval`}>{t("gitlab:pollIntervalSeconds")}</Label>
          <p className="text-xs text-muted-foreground">{t("gitlab:between60And3600Seconds")}</p>
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
          <Label htmlFor={`${kind}-watch-inflight`}>{t("gitlab:maximumInFlightTasks")}</Label>
          <p className="text-xs text-muted-foreground">
            {t("gitlab:optionalCapPollingResumesWhenActive")}
          </p>
          <Input
            id={`${kind}-watch-inflight`}
            type="number"
            min={1}
            value={form.maxInflightTasks}
            onChange={(event) =>
              setForm((current) => ({ ...current, maxInflightTasks: event.target.value }))
            }
            placeholder={t("gitlab:noLimit")}
          />
        </div>
      </div>
      <SelectField
        label={t("gitlab:cleanupPolicy")}
        description={t("gitlab:controlsTaskDeletionWhenTheGitlab")}
        value={form.cleanupPolicy}
        onChange={(value) =>
          setForm((current) => ({
            ...current,
            cleanupPolicy: value as GitLabWatchForm["cleanupPolicy"],
          }))
        }
        placeholder={t("gitlab:auto")}
        items={[
          { id: "auto", label: t("gitlab:autoKeepEngagedTasks") },
          { id: "always", label: t("gitlab:alwaysDelete") },
          { id: "never", label: t("gitlab:neverDelete") },
        ]}
      />
    </div>
  );
}

function dialogTitle(t: TFunction, kind: GitLabWatchKind, editing: boolean): string {
  if (kind === "review") {
    return editing ? t("gitlab:editReviewWatch") : t("gitlab:createReviewWatch");
  }
  return editing ? t("gitlab:editIssueWatch") : t("gitlab:createIssueWatch");
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
  const { t } = useTranslation();
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
      setError(cause instanceof Error ? cause.message : t("gitlab:gitlabWatchCouldNotBeSaved"));
    } finally {
      setSaving(false);
    }
  }, [onCreate, onOpenChange, onUpdate, payload, t, watch]);
  let saveLabel = t("gitlab:createWatch");
  if (watch) saveLabel = t("gitlab:updateWatch");
  if (saving) saveLabel = t("gitlab:saving");
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-none overflow-y-auto sm:max-h-[90vh] sm:w-[min(900px,calc(100vw-2rem))]">
        <DialogHeader>
          {/* Four whole sentences rather than "Edit"/"Create" + a `kind` noun:
              `kind` is the wire discriminant and must never be translated, and
              splicing a translated noun into a translated stem fixes English
              word order. */}
          <DialogTitle>{dialogTitle(t, kind, Boolean(watch))}</DialogTitle>
          <DialogDescription>
            {kind === "review"
              ? t("gitlab:reviewWatchDialogDescription")
              : t("gitlab:issueWatchDialogDescription")}
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
            {t("common:cancel")}
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
