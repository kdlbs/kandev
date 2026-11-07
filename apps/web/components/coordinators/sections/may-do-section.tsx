"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Skeleton } from "@kandev/ui/skeleton";
import AppLink from "@/components/routing/app-link";
import type { ControlAction, ControlSetting } from "@/lib/api/domains/coordinator-api";
import type { ClassEligibility } from "@/lib/api/domains/coordinator-automatic-api";
import { linkToCoordinatorActivityClass } from "@/lib/coordinator/links";
import { ACTION_LABEL_KEY, CONTROL_ACTIONS } from "@/lib/coordinators/control-draft";
import {
  useActionSummary,
  type ActionCounts,
} from "@/hooks/domains/coordinator/use-action-summary";
import type { useControlDraft } from "@/hooks/domains/coordinator/use-control-draft";
import {
  useClassEligibility,
  type ClassEligibilityStatus,
} from "@/hooks/domains/coordinator/use-class-eligibility";
import { useReviewOpened } from "@/hooks/domains/coordinator/use-review-opened";
import { AutomaticEligibility, RaisedRecord } from "./automatic-eligibility";
import { EligibilitySettingsGroup } from "./eligibility-settings";

type Control = ReturnType<typeof useControlDraft>;

type MayDoSectionProps = {
  workspaceId: string;
  coordinatorId: string;
  canManage: boolean;
  control: Control;
  phase3?: boolean;
};

/** The classes the server accepts as automatic. */
const RAISABLE_ACTIONS: ReadonlySet<ControlAction> = new Set([
  "create_task",
  "start_agent",
  "resume",
]);

/** The phase 3 raise surface of one class: its eligibility and the review flow. */
export type MayDoAutomatic = {
  eligibility: ClassEligibility | null;
  status: ClassEligibilityStatus;
  reviewOpened: boolean;
  reviewFailed: boolean;
  onReviewOpened: () => void;
  onMarkReviewed: () => void;
  onRetry: () => void;
};

/** Per-class raise surface; undefined while phase 3 is not effective. */
export type MayDoAutomaticMap = Partial<Record<ControlAction, MayDoAutomatic>>;

function automaticRadioState(
  action: ControlAction,
  value: ControlSetting,
  canManage: boolean,
  automaticMap: MayDoAutomaticMap | undefined,
): { disabled: boolean; noteKey: string } {
  if (!automaticMap) return { disabled: true, noteKey: "coordinator:mayDoAutomaticNote" };
  const automatic = automaticMap[action];
  if (!RAISABLE_ACTIONS.has(action) || !automatic) {
    return { disabled: true, noteKey: "coordinator:mayDoAutomaticCannot" };
  }
  const allowed = canManage && (value === "automatic" || automatic.eligibility?.eligible === true);
  return { disabled: !allowed, noteKey: "coordinator:mayDoAutomaticNotEligible" };
}

function Counts({
  action,
  counts,
  status,
}: {
  action: ControlAction;
  counts: ActionCounts | null;
  status: string;
}) {
  const { t } = useTranslation();
  if (status === "loading")
    return <Skeleton className="h-4 w-40" data-testid="may-do-counts-loading" />;
  const entry = counts?.[action];
  if (status === "error" || !entry) return null;
  if (entry.approved === 0 && entry.rejected === 0) {
    return (
      <span className="text-xs text-muted-foreground">{t("coordinator:mayDoNothingYet")}</span>
    );
  }
  return (
    <span className="text-xs text-muted-foreground" data-testid={`may-do-counts-${action}`}>
      {t("coordinator:mayDoCounts", { approved: entry.approved, rejected: entry.rejected })}
    </span>
  );
}

export type MayDoActivity = {
  workspaceId: string;
  coordinatorId: string;
  counts: ActionCounts | null;
  status: string;
};

type RowProps = {
  action: ControlAction;
  value: ControlSetting;
  disabled: boolean;
  onChange: (value: ControlSetting) => void;
  activity?: MayDoActivity;
  errorMessage?: string;
  automatic?: MayDoAutomaticMap;
};

function AutomaticOption({
  action,
  radio,
}: {
  action: ControlAction;
  radio: { disabled: boolean; noteKey: string };
}) {
  const { t } = useTranslation();
  return (
    <label
      className={`flex items-center gap-2 text-sm ${radio.disabled ? "text-muted-foreground" : "cursor-pointer"}`}
    >
      <RadioGroupItem
        value="automatic"
        id={`may-do-${action}-automatic`}
        disabled={radio.disabled}
      />
      {t("coordinator:mayDoAutomatic")}
      {radio.disabled && (
        <span className="text-xs" data-testid={`may-do-automatic-note-${action}`}>
          {t(radio.noteKey)}
        </span>
      )}
    </label>
  );
}

function ActionRow({
  action,
  value,
  disabled,
  onChange,
  activity,
  errorMessage,
  automatic,
}: RowProps) {
  const { t } = useTranslation();
  const stopLocked = action === "stop";
  const name = `may-do-${action}`;
  const radio = automaticRadioState(action, value, !disabled, automatic);
  const radioValue = value === "automatic" && !automatic ? "denied" : value;
  const entry = automatic?.[action];
  const onReviewClick = entry?.onReviewOpened;
  return (
    <div className="space-y-2 py-3" data-testid={`may-do-row-${action}`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm font-medium">{t(ACTION_LABEL_KEY[action])}</span>
        {activity && (
          <AppLink
            href={linkToCoordinatorActivityClass(
              activity.workspaceId,
              activity.coordinatorId,
              action,
            )}
            className="cursor-pointer text-xs underline"
            onClick={onReviewClick}
            data-testid={`may-do-review-${action}`}
          >
            {t("coordinator:mayDoReviewLast30")}
          </AppLink>
        )}
      </div>
      <RadioGroup
        value={radioValue}
        onValueChange={(next) => onChange(next as ControlSetting)}
        disabled={disabled}
        className="flex flex-wrap gap-4"
        aria-label={t(ACTION_LABEL_KEY[action])}
      >
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <RadioGroupItem value="denied" id={`${name}-denied`} />
          {t("coordinator:mayDoDenied")}
        </label>
        <label className="flex cursor-pointer items-center gap-2 text-sm">
          <RadioGroupItem value="requires_approval" id={`${name}-approval`} disabled={stopLocked} />
          {t("coordinator:mayDoRequiresApproval")}
        </label>
        <AutomaticOption action={action} radio={radio} />
      </RadioGroup>
      {stopLocked && (
        <p className="text-xs text-muted-foreground">{t("coordinator:mayDoStopUnavailable")}</p>
      )}
      {action === "start_agent" && value !== "denied" && (
        <p className="text-xs text-muted-foreground" data-testid="may-do-start-agent-note">
          {t("coordinator:mayDoStartAgentNote")}
        </p>
      )}
      {errorMessage && (
        <p role="alert" className="text-xs text-destructive" data-testid={`may-do-error-${action}`}>
          {errorMessage}
        </p>
      )}
      {activity && <Counts action={action} counts={activity.counts} status={activity.status} />}
      {entry && (
        <>
          {entry.eligibility && <RaisedRecord eligibility={entry.eligibility} />}
          <AutomaticEligibility
            eligibility={entry.eligibility}
            status={entry.status}
            canManage={!disabled}
            reviewOpened={entry.reviewOpened}
            reviewFailed={entry.reviewFailed}
            onMarkReviewed={entry.onMarkReviewed}
            onRetry={entry.onRetry}
          />
        </>
      )}
    </div>
  );
}

export type MayDoRowsProps = {
  actions: Record<ControlAction, ControlSetting>;
  canManage: boolean;
  onChange: (action: ControlAction, value: ControlSetting) => void;
  activity?: MayDoActivity;
  rowErrors?: Partial<Record<ControlAction, string>>;
  errorMessage?: string | null;
  freshConversationNote?: boolean;
  automatic?: MayDoAutomaticMap;
};

export function MayDoRows({
  actions,
  canManage,
  onChange,
  activity,
  rowErrors,
  errorMessage,
  freshConversationNote = false,
  automatic,
}: MayDoRowsProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" data-testid="may-do-section">
      {errorMessage && (
        <p role="alert" className="text-sm text-destructive" data-testid="may-do-error">
          {errorMessage}
        </p>
      )}
      <div className="divide-y">
        {CONTROL_ACTIONS.map((action) => (
          <ActionRow
            key={action}
            action={action}
            value={actions[action]}
            disabled={!canManage}
            onChange={(next) => onChange(action, next)}
            activity={activity}
            errorMessage={rowErrors?.[action]}
            automatic={automatic}
          />
        ))}
      </div>
      <div className="space-y-1 pt-3 text-sm" data-testid="may-do-always-human">
        <p className="font-medium">{t("coordinator:mayDoAlwaysHuman")}</p>
        <p className="text-muted-foreground">{t("coordinator:mayDoMergePr")}</p>
        <p className="text-muted-foreground">{t("coordinator:mayDoMoveToDone")}</p>
      </div>
      {freshConversationNote && (
        <p className="text-xs text-muted-foreground">{t("coordinator:mayDoFreshConversation")}</p>
      )}
    </div>
  );
}

function useAutomaticClass(
  workspaceId: string,
  coordinatorId: string,
  action: ControlAction,
  phase3: boolean,
): MayDoAutomatic {
  const eligibility = useClassEligibility(workspaceId, coordinatorId, action, phase3);
  const [reviewOpened, markReviewOpened] = useReviewOpened(coordinatorId, action);
  return {
    eligibility: eligibility.eligibility,
    status: eligibility.status,
    reviewOpened,
    reviewFailed: eligibility.reviewFailed,
    onReviewOpened: markReviewOpened,
    onMarkReviewed: eligibility.markReviewed,
    onRetry: eligibility.reload,
  };
}

function useAutomaticMap(
  workspaceId: string,
  coordinatorId: string,
  phase3: boolean,
): MayDoAutomaticMap | undefined {
  const createTask = useAutomaticClass(workspaceId, coordinatorId, "create_task", phase3);
  const startAgent = useAutomaticClass(workspaceId, coordinatorId, "start_agent", phase3);
  const resume = useAutomaticClass(workspaceId, coordinatorId, "resume", phase3);
  return phase3 ? { create_task: createTask, start_agent: startAgent, resume } : undefined;
}

/** A 400 naming `policy.eligibility` shows above the group, not on an action row. */
function eligibilityError(control: Control): string | null {
  const error = control.fieldError;
  return error?.field?.startsWith("policy.eligibility") ? error.detail : null;
}

export function MayDoSection({
  workspaceId,
  coordinatorId,
  canManage,
  control,
  phase3 = false,
}: MayDoSectionProps) {
  const { t } = useTranslation();
  const summary = useActionSummary(workspaceId, coordinatorId, 30);
  const automatic = useAutomaticMap(workspaceId, coordinatorId, phase3);
  const { draft, status, retry } = control;

  if (status === "loading" || !draft) {
    if (status === "error") {
      return (
        <div className="flex items-center gap-2 text-sm" data-testid="may-do-load-failed">
          {t("coordinator:mayDoLoadFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={retry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      );
    }
    return <Skeleton className="h-48 w-full" data-testid="may-do-loading" />;
  }

  return (
    <div className="space-y-2">
      {summary.status === "error" && (
        <div className="flex items-center gap-2 text-sm" data-testid="may-do-summary-failed">
          {t("coordinator:mayDoSummaryFailed")}
          <Button variant="outline" size="sm" className="cursor-pointer" onClick={summary.retry}>
            {t("coordinator:tryAgain")}
          </Button>
        </div>
      )}
      <MayDoRows
        actions={draft.actions}
        canManage={canManage}
        onChange={control.setAction}
        activity={{ workspaceId, coordinatorId, counts: summary.counts, status: summary.status }}
        freshConversationNote
        automatic={automatic}
      />
      {draft.eligibility && (
        <EligibilitySettingsGroup
          eligibility={draft.eligibility}
          canManage={canManage}
          serverError={eligibilityError(control)}
          onChange={control.setEligibility}
        />
      )}
    </div>
  );
}
