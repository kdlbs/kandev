"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import type {
  TaskCompletionCriterionInput,
  TaskCompletionEvidenceSubject,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";

const EVIDENCE_KINDS = [
  "task_revision",
  "execution",
  "artifact_revision",
  "github_pr_head",
] as const;

export type CompletionDraftCriterion = TaskCompletionCriterionInput;

export function toCompletionDraft(snapshot: TaskCompletionGateSnapshot | null) {
  return (snapshot?.criteria ?? []).map((criterion) => ({
    id: criterion.id,
    description: criterion.description,
    evidence_subject: {
      kind: criterion.evidence_subject.kind,
      id: criterion.evidence_subject.id,
    },
  }));
}

export function completionDraftWeakensUnmetCriteria(
  snapshot: TaskCompletionGateSnapshot | null,
  draft: CompletionDraftCriterion[],
) {
  if (!snapshot) return false;
  const blockers = new Set((snapshot.blockers ?? []).map((blocker) => blocker.criterion_id));
  return snapshot.criteria.some((current) => {
    if (!blockers.has(current.id)) return false;
    const next = draft.find((criterion) => criterion.id === current.id);
    return (
      !next ||
      next.description !== current.description ||
      next.evidence_subject.kind !== current.evidence_subject.kind ||
      next.evidence_subject.id !== current.evidence_subject.id
    );
  });
}

export function completionCriterionIsVerified(
  criterion: TaskCompletionGateSnapshot["criteria"][number],
) {
  return criterion.verified_revision === criterion.criterion_revision && criterion.evidence != null;
}

type CriteriaSectionProps = {
  snapshot: TaskCompletionGateSnapshot | null;
  taskUpdatedAt: string;
  draft: CompletionDraftCriterion[];
  onDraftChange: (draft: CompletionDraftCriterion[]) => void;
  savingCriteria: boolean;
  savingEvidence: string | null;
  changed: boolean;
  weakening: boolean;
  confirmWeakening: boolean;
  onConfirmWeakeningChange: (value: boolean) => void;
  confirmationReason: string;
  onConfirmationReasonChange: (value: string) => void;
  onSaveCriteria: () => void;
  onSaveEvidence: (
    criterionId: string,
    subject: TaskCompletionEvidenceSubject,
    revision: string,
    summary: string,
    reference: string,
  ) => Promise<void>;
};

export function TaskCompletionCriteriaSection(props: CriteriaSectionProps) {
  const { t } = useTranslation();
  const verified = props.snapshot?.criteria.filter(completionCriterionIsVerified).length ?? 0;
  const total = props.snapshot?.criteria.length ?? 0;
  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="task-completion-gate-criteria-heading"
    >
      <div className="flex items-center justify-between gap-3">
        <h3 id="task-completion-gate-criteria-heading" className="text-sm font-medium">
          {t("task:completionGateCriteria")}
        </h3>
        <span className="text-xs text-muted-foreground">
          {t("task:completionGateProgress", { verified, total })}
        </span>
      </div>
      <CompletionCriterionList
        snapshot={props.snapshot}
        taskUpdatedAt={props.taskUpdatedAt}
        savingEvidence={props.savingEvidence}
        onSaveEvidence={props.onSaveEvidence}
      />
      <AddCriterionForm
        draft={props.draft}
        saving={props.savingCriteria}
        onDraftChange={props.onDraftChange}
      />
      {props.weakening && (
        <WeakeningConfirmation
          checked={props.confirmWeakening}
          reason={props.confirmationReason}
          onCheckedChange={props.onConfirmWeakeningChange}
          onReasonChange={props.onConfirmationReasonChange}
        />
      )}
      <Button
        type="button"
        className="min-h-11"
        disabled={
          !props.snapshot ||
          !props.changed ||
          props.savingCriteria ||
          (props.weakening && (!props.confirmWeakening || !props.confirmationReason.trim()))
        }
        onClick={props.onSaveCriteria}
        data-testid="task-completion-gate-save-criteria"
      >
        {props.savingCriteria
          ? t("task:completionGateSaving")
          : t("task:completionGateSaveCriteria")}
      </Button>
    </section>
  );
}

type CompletionCriterionListProps = Pick<
  CriteriaSectionProps,
  "snapshot" | "taskUpdatedAt" | "savingEvidence" | "onSaveEvidence"
>;

function CompletionCriterionList(props: CompletionCriterionListProps) {
  const { t } = useTranslation();
  return (
    <ul className="flex flex-col gap-3" data-testid="task-completion-gate-criteria-list">
      {(props.snapshot?.criteria ?? []).map((criterion) => (
        <CompletionCriterionCard
          key={criterion.id}
          criterion={criterion}
          taskUpdatedAt={props.taskUpdatedAt}
          savingEvidence={props.savingEvidence}
          blockers={props.snapshot?.blockers ?? []}
          onSaveEvidence={props.onSaveEvidence}
        />
      ))}
      {props.snapshot && props.snapshot.criteria.length === 0 && (
        <li className="text-sm text-muted-foreground">{t("task:completionGateNoCriteria")}</li>
      )}
    </ul>
  );
}

type CriterionCardProps = {
  criterion: TaskCompletionGateSnapshot["criteria"][number];
  taskUpdatedAt: string;
  savingEvidence: string | null;
  blockers: NonNullable<TaskCompletionGateSnapshot["blockers"]>;
  onSaveEvidence: CriteriaSectionProps["onSaveEvidence"];
};

function CompletionCriterionCard(props: CriterionCardProps) {
  const { t } = useTranslation();
  const verified = completionCriterionIsVerified(props.criterion);
  const blocked = props.blockers.some((item) => item.criterion_id === props.criterion.id);
  const statusClass = blocked
    ? "shrink-0 text-xs text-destructive"
    : "shrink-0 text-xs text-emerald-600";
  return (
    <li
      className="rounded-md border border-border p-3"
      data-testid="task-completion-gate-criterion"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="break-words text-sm font-medium">{props.criterion.description}</p>
          <p className="mt-1 break-all text-xs text-muted-foreground">
            {t(`task:completionGateEvidenceType_${props.criterion.evidence_subject.kind}`)}:{" "}
            {props.criterion.evidence_subject.id}
          </p>
        </div>
        <span className={statusClass}>
          {verified && !blocked
            ? t("task:completionGateVerified")
            : t("task:completionGateNeedsEvidence")}
        </span>
      </div>
      {props.criterion.evidence && (
        <p className="mt-2 break-words text-xs text-muted-foreground">
          {props.criterion.evidence.summary ||
            props.criterion.evidence.reference ||
            props.criterion.evidence.subject.revision}
        </p>
      )}
      <TaskCompletionEvidenceForm
        criterionId={props.criterion.id}
        subject={props.criterion.evidence_subject}
        taskUpdatedAt={props.taskUpdatedAt}
        busy={props.savingEvidence !== null}
        saving={props.savingEvidence === props.criterion.id}
        onSubmit={props.onSaveEvidence}
      />
    </li>
  );
}

type AddCriterionFormProps = {
  draft: CompletionDraftCriterion[];
  saving: boolean;
  onDraftChange: (draft: CompletionDraftCriterion[]) => void;
};

function AddCriterionForm({ draft, saving, onDraftChange }: AddCriterionFormProps) {
  const { t } = useTranslation();
  const [id, setId] = useState("");
  const [description, setDescription] = useState("");
  const [subjectKind, setSubjectKind] =
    useState<(typeof EVIDENCE_KINDS)[number]>("artifact_revision");
  const [subjectId, setSubjectId] = useState("");
  const add = () => {
    const criterionId = id.trim();
    const criterionDescription = description.trim();
    const evidenceSubjectId = subjectId.trim();
    if (!criterionId || !criterionDescription || !evidenceSubjectId) return;
    onDraftChange([
      ...draft.filter((criterion) => criterion.id !== criterionId),
      {
        id: criterionId,
        description: criterionDescription,
        evidence_subject: { kind: subjectKind, id: evidenceSubjectId },
      },
    ]);
    setId("");
    setDescription("");
    setSubjectId("");
  };
  return (
    <fieldset className="flex flex-col gap-3 rounded-md border border-border p-3" disabled={saving}>
      <legend className="px-1 text-xs font-medium text-muted-foreground">
        {t("task:completionGateAddCriterion")}
      </legend>
      <label className="flex flex-col gap-1 text-sm" htmlFor="task-completion-gate-new-id">
        {t("task:completionGateCriterionId")}
        <input
          id="task-completion-gate-new-id"
          className="min-h-11 rounded-md border border-input bg-background px-3"
          value={id}
          onChange={(event) => setId(event.currentTarget.value)}
          data-testid="task-completion-gate-new-id"
        />
      </label>
      <label className="flex flex-col gap-1 text-sm" htmlFor="task-completion-gate-new-description">
        {t("task:completionGateDescription")}
        <Textarea
          id="task-completion-gate-new-description"
          className="min-h-16 resize-y"
          value={description}
          onChange={(event) => setDescription(event.currentTarget.value)}
          data-testid="task-completion-gate-new-description"
        />
      </label>
      <EvidenceSubjectFields
        kind={subjectKind}
        subjectId={subjectId}
        onKindChange={setSubjectKind}
        onSubjectIdChange={setSubjectId}
      />
      <Button
        type="button"
        variant="outline"
        className="min-h-11"
        disabled={!id.trim() || !description.trim() || !subjectId.trim()}
        onClick={add}
        data-testid="task-completion-gate-add-criterion"
      >
        {t("task:completionGateAddCriterion")}
      </Button>
    </fieldset>
  );
}

type EvidenceSubjectFieldsProps = {
  kind: (typeof EVIDENCE_KINDS)[number];
  subjectId: string;
  onKindChange: (kind: (typeof EVIDENCE_KINDS)[number]) => void;
  onSubjectIdChange: (id: string) => void;
};

function EvidenceSubjectFields(props: EvidenceSubjectFieldsProps) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <label className="flex flex-col gap-1 text-sm" htmlFor="task-completion-gate-new-kind">
        {t("task:completionGateEvidenceType")}
        <select
          id="task-completion-gate-new-kind"
          className="min-h-11 rounded-md border border-input bg-background px-3"
          value={props.kind}
          onChange={(event) =>
            props.onKindChange(event.currentTarget.value as (typeof EVIDENCE_KINDS)[number])
          }
          data-testid="task-completion-gate-new-kind"
        >
          {EVIDENCE_KINDS.map((kind) => (
            <option key={kind} value={kind}>
              {t(`task:completionGateEvidenceType_${kind}`)}
            </option>
          ))}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm" htmlFor="task-completion-gate-new-subject">
        {t("task:completionGateEvidenceSubjectId")}
        <input
          id="task-completion-gate-new-subject"
          className="min-h-11 rounded-md border border-input bg-background px-3"
          value={props.subjectId}
          onChange={(event) => props.onSubjectIdChange(event.currentTarget.value)}
          data-testid="task-completion-gate-new-subject"
        />
      </label>
    </div>
  );
}

type WeakeningConfirmationProps = {
  checked: boolean;
  reason: string;
  onCheckedChange: (checked: boolean) => void;
  onReasonChange: (reason: string) => void;
};

function WeakeningConfirmation(props: WeakeningConfirmationProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3 rounded-md border border-amber-500/40 bg-amber-500/5 p-3">
      <p className="text-sm">{t("task:completionGateHumanConfirmation")}</p>
      <label
        className="flex items-start gap-2 text-sm"
        htmlFor="task-completion-gate-confirm-weakening"
      >
        <input
          id="task-completion-gate-confirm-weakening"
          type="checkbox"
          className="mt-1 size-4"
          checked={props.checked}
          onChange={(event) => props.onCheckedChange(event.currentTarget.checked)}
          data-testid="task-completion-gate-confirm-weakening"
        />
        <span>{t("task:completionGateConfirmWeakening")}</span>
      </label>
      <label
        className="flex flex-col gap-1 text-sm"
        htmlFor="task-completion-gate-confirmation-reason"
      >
        {t("task:completionGateConfirmationReason")}
        <Textarea
          id="task-completion-gate-confirmation-reason"
          className="min-h-16 resize-y"
          value={props.reason}
          onChange={(event) => props.onReasonChange(event.currentTarget.value)}
          data-testid="task-completion-gate-confirmation-reason"
        />
      </label>
    </div>
  );
}

type EvidenceFormProps = {
  criterionId: string;
  subject: TaskCompletionEvidenceSubject;
  taskUpdatedAt: string;
  busy: boolean;
  saving: boolean;
  onSubmit: CriteriaSectionProps["onSaveEvidence"];
};

function TaskCompletionEvidenceForm(props: EvidenceFormProps) {
  const { t } = useTranslation();
  const [revision, setRevision] = useState(
    props.subject.kind === "task_revision" ? props.taskUpdatedAt : "",
  );
  const [summary, setSummary] = useState("");
  const [reference, setReference] = useState("");
  useEffect(() => {
    if (props.subject.kind === "task_revision") setRevision(props.taskUpdatedAt);
  }, [props.subject.kind, props.taskUpdatedAt]);
  return (
    <div className="mt-3 flex flex-col gap-2 border-t border-border pt-3">
      <label
        className="flex flex-col gap-1 text-xs"
        htmlFor={`task-completion-evidence-revision-${props.criterionId}`}
      >
        {t("task:completionGateEvidenceRevision")}
        <input
          id={`task-completion-evidence-revision-${props.criterionId}`}
          className="min-h-11 rounded-md border border-input bg-background px-3 text-sm"
          value={revision}
          onChange={(event) => setRevision(event.currentTarget.value)}
          data-testid={`task-completion-evidence-revision-${props.criterionId}`}
        />
      </label>
      <label
        className="flex flex-col gap-1 text-xs"
        htmlFor={`task-completion-evidence-summary-${props.criterionId}`}
      >
        {t("task:completionGateEvidenceSummary")}
        <Textarea
          id={`task-completion-evidence-summary-${props.criterionId}`}
          className="min-h-16 resize-y text-sm"
          value={summary}
          onChange={(event) => setSummary(event.currentTarget.value)}
          data-testid={`task-completion-evidence-summary-${props.criterionId}`}
        />
      </label>
      <label
        className="flex flex-col gap-1 text-xs"
        htmlFor={`task-completion-evidence-reference-${props.criterionId}`}
      >
        {t("task:completionGateEvidenceReference")}
        <input
          id={`task-completion-evidence-reference-${props.criterionId}`}
          className="min-h-11 rounded-md border border-input bg-background px-3 text-sm"
          value={reference}
          onChange={(event) => setReference(event.currentTarget.value)}
          data-testid={`task-completion-evidence-reference-${props.criterionId}`}
        />
      </label>
      <Button
        type="button"
        variant="outline"
        className="min-h-11"
        disabled={!revision.trim() || props.busy}
        onClick={() =>
          void props.onSubmit(props.criterionId, props.subject, revision, summary, reference)
        }
        data-testid={`task-completion-evidence-submit-${props.criterionId}`}
      >
        {props.saving ? t("task:completionGateSaving") : t("task:completionGateVerify")}
      </Button>
    </div>
  );
}
