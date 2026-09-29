"use client";

import { useEffect, useMemo, useRef, useState, type FormEvent, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Field, FieldContent, FieldDescription, FieldError, FieldLabel } from "@kandev/ui/field";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";
import type { ApproveProposalEdits, ProposalSpec } from "@/lib/api/domains/coordinator-api";
import type { Repository, Workflow, WorkflowStepDTO } from "@/lib/types/http";
import {
  useProposalEditOptions,
  type OptionsField,
} from "@/hooks/domains/coordinator/use-proposal-edit-options";

export type EditFormServerError = { message: string; field: string | null };

export type EditFormProps = {
  workspaceId: string;
  spec: ProposalSpec;
  busy: boolean;
  serverError: EditFormServerError | null;
  onApprove: (edits: ApproveProposalEdits) => void;
  onCancel: () => void;
};

const NO_REPOSITORY = "__none__";

/** Element ids of the form's fields, keyed by the approve route's `field` names. */
const fieldElementIds: Record<string, string> = {
  title: "proposal-edit-title", // i18n-exempt: DOM id, not copy
  description: "proposal-edit-description", // i18n-exempt: DOM id, not copy
  workflow_id: "proposal-edit-workflow", // i18n-exempt: DOM id, not copy
  step_id: "proposal-edit-step", // i18n-exempt: DOM id, not copy
  repository_id: "proposal-edit-repository", // i18n-exempt: DOM id, not copy
};

/** The server error's field when it names a field on this form, else null. */
function formErrorField(serverError: EditFormServerError | null): string | null {
  const field = serverError?.field;
  return field && field in fieldElementIds ? field : null;
}

function LoadFailed({ busy, retry }: { busy: boolean; retry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <FieldError>{t("coordinator:editOptionsLoadFailed")}</FieldError>
      <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={retry}>
        {t("coordinator:tryAgain")}
      </Button>
    </div>
  );
}

type WorkflowFieldProps = {
  workflows: OptionsField<Workflow>;
  workflowId: string;
  /** The selected workflow's name from its snapshot, shown while the list read has failed. */
  fallbackName: string | null;
  busy: boolean;
  onChange: (id: string) => void;
  retry: () => void;
};

function WorkflowField({
  workflows,
  workflowId,
  fallbackName,
  busy,
  onChange,
  retry,
}: WorkflowFieldProps) {
  const { t } = useTranslation();
  const id = fieldElementIds.workflow_id;
  if (workflows.status === "loading") {
    return <Input id={id} disabled value={t("coordinator:editOptionsLoading")} />;
  }
  if (workflows.status === "error") {
    return (
      <div className="space-y-1">
        <Input id={id} disabled value={fallbackName ?? ""} />
        <LoadFailed busy={busy} retry={retry} />
      </div>
    );
  }
  const currentKnown = workflows.value.some((w) => w.id === workflowId);
  return (
    <div className="space-y-1">
      <Select value={workflowId || undefined} onValueChange={onChange} disabled={busy}>
        <SelectTrigger id={id}>
          <SelectValue placeholder={t("coordinator:editSelectWorkflow")} />
        </SelectTrigger>
        <SelectContent>
          {workflows.value.map((w) => (
            <SelectItem key={w.id} value={w.id}>
              {w.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {!currentKnown && <FieldDescription>{t("coordinator:editWorkflowMissing")}</FieldDescription>}
    </div>
  );
}

type StepFieldProps = {
  /** False while the selected workflow is not (yet) known to exist: the field stays empty. */
  workflowResolved: boolean;
  steps: OptionsField<WorkflowStepDTO>;
  stepId: string;
  busy: boolean;
  onChange: (id: string) => void;
  retry: () => void;
};

function StepField({ workflowResolved, steps, stepId, busy, onChange, retry }: StepFieldProps) {
  const { t } = useTranslation();
  const id = fieldElementIds.step_id;
  if (!workflowResolved) {
    return <Input id={id} disabled value="" />;
  }
  if (steps.status === "loading") {
    return <Input id={id} disabled value={t("coordinator:editOptionsLoading")} />;
  }
  if (steps.status === "error") {
    return (
      <div className="space-y-1">
        <Input id={id} disabled value="" />
        <LoadFailed busy={busy} retry={retry} />
      </div>
    );
  }
  if (steps.value.length === 0) {
    return (
      <div className="space-y-1">
        <Input id={id} disabled value="" />
        <FieldDescription>{t("coordinator:editNoEligibleStep")}</FieldDescription>
      </div>
    );
  }
  const stepMissing = stepId !== "" && !steps.value.some((s) => s.id === stepId);
  return (
    <div className="space-y-1">
      <Select
        value={stepMissing ? undefined : stepId || undefined}
        onValueChange={onChange}
        disabled={busy}
      >
        <SelectTrigger id={id}>
          <SelectValue placeholder={t("coordinator:editSelectStep")} />
        </SelectTrigger>
        <SelectContent>
          {steps.value.map((s) => (
            <SelectItem key={s.id} value={s.id}>
              {s.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {stepMissing && <FieldDescription>{t("coordinator:editStepMissing")}</FieldDescription>}
    </div>
  );
}

type RepositoryFieldProps = {
  repositories: OptionsField<Repository>;
  repositoryId: string;
  busy: boolean;
  onChange: (id: string) => void;
  retry: () => void;
};

function RepositoryField({
  repositories,
  repositoryId,
  busy,
  onChange,
  retry,
}: RepositoryFieldProps) {
  const { t } = useTranslation();
  const id = fieldElementIds.repository_id;
  if (repositories.status === "loading") {
    return <Input id={id} disabled value={t("coordinator:editOptionsLoading")} />;
  }
  const currentUnavailable =
    repositoryId !== "" && !repositories.value.some((r) => r.id === repositoryId);
  return (
    <div className="space-y-1">
      <Select value={repositoryId || NO_REPOSITORY} onValueChange={onChange} disabled={busy}>
        <SelectTrigger id={id}>
          <SelectValue placeholder={t("coordinator:editSelectRepository")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NO_REPOSITORY}>{t("coordinator:editNoRepository")}</SelectItem>
          {repositories.value.map((r) => (
            <SelectItem key={r.id} value={r.id}>
              {r.name}
            </SelectItem>
          ))}
          {currentUnavailable && (
            <SelectItem value={repositoryId}>
              {t("coordinator:editUnavailableRepository")}
            </SelectItem>
          )}
        </SelectContent>
      </Select>
      {repositories.status === "error" && <LoadFailed busy={busy} retry={retry} />}
    </div>
  );
}

function TitleFieldError({
  titleError,
  serverError,
}: {
  titleError: string | null;
  serverError: EditFormServerError | null;
}) {
  const message = titleError ?? (serverError?.field === "title" ? serverError.message : null);
  if (!message) return null;
  return <FieldError>{message}</FieldError>;
}

function FieldServerError({
  serverError,
  field,
}: {
  serverError: EditFormServerError | null;
  field: string;
}) {
  if (!serverError || formErrorField(serverError) !== field) return null;
  return <FieldError>{serverError.message}</FieldError>;
}

function GeneralServerError({
  serverError,
  alertRef,
}: {
  serverError: EditFormServerError | null;
  alertRef: RefObject<HTMLDivElement | null>;
}) {
  if (!serverError || formErrorField(serverError)) return null;
  return (
    <div
      ref={alertRef}
      role="alert"
      tabIndex={-1}
      className="text-destructive text-xs/relaxed font-normal"
    >
      {serverError.message}
    </div>
  );
}

function changedEdits(
  spec: ProposalSpec,
  values: {
    title: string;
    description: string;
    workflowId: string;
    stepId: string;
    repositoryId: string;
  },
): ApproveProposalEdits {
  const edits: ApproveProposalEdits = {};
  if (values.title !== spec.title) edits.title = values.title;
  if (values.description !== spec.description) edits.description = values.description;
  if (values.workflowId !== spec.workflow_id) edits.workflow_id = values.workflowId;
  if (values.stepId !== spec.step_id) edits.step_id = values.stepId;
  if (values.repositoryId !== spec.repository_id) edits.repository_id = values.repositoryId;
  return edits;
}

/**
 * The Edit form: title, description, workflow, step (eligible only) and
 * repository, with Approve with edits and Cancel
 * (docs/specs/coordinator/requirements/proposals.md
 * AC-COORDINATOR-PROPOSALS-005.4, docs/specs/coordinator/system-design/
 * proposal-cards.md#cards "Edit form options").
 */
// eslint-disable-next-line max-lines-per-function -- one form owns five fields' loading/error/missing-value states
export function EditForm({
  workspaceId,
  spec,
  busy,
  serverError,
  onApprove,
  onCancel,
}: EditFormProps) {
  const { t } = useTranslation();
  const [title, setTitle] = useState(spec.title);
  const [description, setDescription] = useState(spec.description);
  const [workflowId, setWorkflowId] = useState(spec.workflow_id);
  const [stepId, setStepId] = useState(spec.step_id);
  const [repositoryId, setRepositoryId] = useState(spec.repository_id);
  const [titleError, setTitleError] = useState<string | null>(null);
  const titleRef = useRef<HTMLInputElement>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  const options = useProposalEditOptions(workspaceId, workflowId);
  const workflowKnown =
    options.workflows.status === "loaded" &&
    options.workflows.value.some((w) => w.id === workflowId);
  // A failed workflow list keeps the current workflow selected and shows its
  // snapshot outcome as usual (proposal-cards.md#cards "Edit form options").
  const workflowResolved = workflowKnown || options.workflows.status === "error";

  // Changing the workflow resets the step to its start step when eligible,
  // else empty (proposal-cards.md#cards "Edit form options").
  const isInitialWorkflow = workflowId === spec.workflow_id;
  useEffect(() => {
    if (isInitialWorkflow || options.steps.status !== "loaded") return;
    const start = options.steps.value.find((s) => s.is_start_step);
    setStepId(start?.id ?? "");
    // Only reacts to a fresh steps load for a workflow the user just picked.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [options.steps.status, workflowId]);

  useEffect(() => {
    titleRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!serverError) return;
    const field = formErrorField(serverError);
    if (field) {
      document.getElementById(fieldElementIds[field])?.focus();
    } else {
      alertRef.current?.focus();
    }
  }, [serverError]);

  // Approve with edits needs a known workflow and a step among its eligible
  // steps; a deleted workflow or an ineligible current step keeps it disabled
  // until the user chooses one.
  const approveDisabled = useMemo(() => {
    if (busy || !workflowKnown || options.steps.status !== "loaded") return true;
    return !options.steps.value.some((s) => s.id === stepId);
  }, [busy, workflowKnown, options.steps, stepId]);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (approveDisabled) return;
    if (!title.trim()) {
      setTitleError(t("coordinator:editTitleRequired"));
      titleRef.current?.focus();
      return;
    }
    setTitleError(null);
    onApprove(
      changedEdits(spec, {
        title: title.trim(),
        description,
        workflowId,
        stepId,
        repositoryId,
      }),
    );
  }

  return (
    <form className="space-y-3" onSubmit={handleSubmit}>
      <Field>
        <FieldContent>
          <FieldLabel htmlFor={fieldElementIds.title}>{t("coordinator:editTitleLabel")}</FieldLabel>
          <Input
            id={fieldElementIds.title}
            ref={titleRef}
            value={title}
            disabled={busy}
            onChange={(e) => setTitle(e.target.value)}
          />
          <TitleFieldError titleError={titleError} serverError={serverError} />
        </FieldContent>
      </Field>
      <Field>
        <FieldContent>
          <FieldLabel htmlFor={fieldElementIds.description}>
            {t("coordinator:editDescriptionLabel")}
          </FieldLabel>
          <Textarea
            id={fieldElementIds.description}
            value={description}
            disabled={busy}
            onChange={(e) => setDescription(e.target.value)}
          />
          <FieldServerError serverError={serverError} field="description" />
        </FieldContent>
      </Field>
      <Field>
        <FieldContent>
          <FieldLabel htmlFor={fieldElementIds.workflow_id}>
            {t("coordinator:editWorkflowLabel")}
          </FieldLabel>
          <WorkflowField
            workflows={options.workflows}
            workflowId={workflowId}
            fallbackName={options.snapshotWorkflowName}
            busy={busy}
            onChange={setWorkflowId}
            retry={options.retryWorkflows}
          />
          <FieldServerError serverError={serverError} field="workflow_id" />
        </FieldContent>
      </Field>
      <Field>
        <FieldContent>
          <FieldLabel htmlFor={fieldElementIds.step_id}>
            {t("coordinator:editStepLabel")}
          </FieldLabel>
          <StepField
            workflowResolved={workflowResolved}
            steps={options.steps}
            stepId={stepId}
            busy={busy}
            onChange={setStepId}
            retry={options.retrySteps}
          />
          <FieldServerError serverError={serverError} field="step_id" />
        </FieldContent>
      </Field>
      <Field>
        <FieldContent>
          <FieldLabel htmlFor={fieldElementIds.repository_id}>
            {t("coordinator:editRepositoryLabel")}
          </FieldLabel>
          <RepositoryField
            repositories={options.repositories}
            repositoryId={repositoryId}
            busy={busy}
            onChange={(id) => setRepositoryId(id === NO_REPOSITORY ? "" : id)}
            retry={options.retryRepositories}
          />
          <FieldServerError serverError={serverError} field="repository_id" />
        </FieldContent>
      </Field>
      <GeneralServerError serverError={serverError} alertRef={alertRef} />
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={approveDisabled} className="min-h-11 sm:min-h-0">
          {busy && <Spinner aria-hidden className="mr-1.5" />}
          {t("coordinator:approveWithEdits")}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={onCancel}
          className="min-h-11 sm:min-h-0"
        >
          {t("coordinator:cancel")}
        </Button>
      </div>
    </form>
  );
}
