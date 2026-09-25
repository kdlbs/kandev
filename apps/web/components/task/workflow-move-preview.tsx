"use client";

import { Button } from "@kandev/ui/button";
import { cn } from "@kandev/ui/lib/utils";
import { IconInfoCircle, IconLoader2, IconRefresh } from "@tabler/icons-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { WorkflowMovePreviewApplicability, WorkflowMovePreviewResponse } from "@/lib/api";
import type { WorkflowMovePreviewState } from "@/hooks/domains/kanban/use-workflow-move-preview";

type Translation = ReturnType<typeof useTranslation>["t"];

const UNKNOWN_PREVIEW_LABEL_KEY = "task:workflowMovePreviewUnknown";

// i18n-exempt: these are closed backend field and value codes used to select
// localized preview copy, never user-facing text.
const PREVIEW_FIELD_TRANSLATION_KEYS: Record<string, string> = {
  mode: "workflowMovePreviewFieldMode",
  step_prompt: "workflowMovePreviewFieldStepPrompt",
  reasoning_effort: "workflowMovePreviewFieldReasoningEffort",
  verbosity: "workflowMovePreviewFieldVerbosity",
};

// i18n-exempt: these are closed backend setting values used to select
// localized preview copy, never user-facing text.
const PREVIEW_VALUE_TRANSLATION_KEYS: Record<string, Record<string, string>> = {
  step_prompt: {
    configured: "workflowMovePreviewValueConfigured",
    skipped: "workflowMovePreviewValueSkipped",
  },
  mode: {
    default: "workflowMovePreviewValueModeDefault",
    plan: "workflowMovePreviewValueModePlan",
  },
  reasoning_effort: {
    low: "workflowMovePreviewValueLow",
    medium: "workflowMovePreviewValueMedium",
    high: "workflowMovePreviewValueHigh",
    max: "workflowMovePreviewValueMax",
  },
  verbosity: {
    low: "workflowMovePreviewValueLow",
    medium: "workflowMovePreviewValueMedium",
    high: "workflowMovePreviewValueHigh",
  },
};

// i18n-exempt: these are closed backend diagnostic codes used to select
// localized preview copy, never user-facing text.
const PREVIEW_NOTICE_TRANSLATION_KEYS: Record<string, string> = {
  retained_model_override: "workflowMovePreviewNoticeRetainedOverride",
  missing_original_snapshot: "workflowMovePreviewNoticeMissingOriginal",
  ambiguous_session_configuration: "workflowMovePreviewNoticeAmbiguous",
  conflicting_session_configuration: "workflowMovePreviewNoticeConflicting",
  session_configuration_skipped: "workflowMovePreviewNoticeInapplicable",
  session_configuration_unavailable: "workflowMovePreviewNoticeConfigurationUnavailable",
  invalid_session_configuration: "workflowMovePreviewNoticeInvalidConfiguration",
  target_unavailable: "workflowMovePreviewNoticeTargetUnavailable",
  profile_unavailable: "workflowMovePreviewNoticeProfileUnavailable",
};

export type WorkflowMovePreviewDisclosureProps = {
  state: WorkflowMovePreviewState;
  isTouchSurface: boolean;
  className?: string;
};

export function workflowMovePreviewChangeCount(preview: WorkflowMovePreviewResponse): number {
  const fieldChanges = (preview.changes ?? []).filter(
    (change) =>
      change.key !== "model" &&
      (change.applicability === "planned" || change.applicability === "unknown"),
  ).length;
  const contextReset =
    preview.context_reset &&
    (preview.context_reset_state === "planned" || preview.context_reset_state === "unknown");
  return fieldChanges + (contextReset ? 1 : 0);
}

function outcomeLabel(t: Translation, preview: WorkflowMovePreviewResponse): string {
  switch (preview.outcome) {
    case "reuse_current":
      return t("task:workflowMovePreviewReuseCurrent");
    case "reuse_other":
      return preview.recipient?.session_name
        ? t("task:workflowMovePreviewReuseOtherNamed", {
            session: preview.recipient.session_name,
          })
        : t("task:workflowMovePreviewReuseOther");
    case "create_new":
      return t("task:workflowMovePreviewNewSession");
    case "no_session":
      return t("task:workflowMovePreviewNoSession");
    default:
      return t("task:workflowMovePreviewUnknownSession");
  }
}

function modelSummaryParts(t: Translation, preview: WorkflowMovePreviewResponse) {
  const after = preview.model.after;
  const before = preview.model.before;
  const modelUnknown = !after.known || !after.label;
  const changeCount = workflowMovePreviewChangeCount(preview);
  if (modelUnknown) {
    return {
      label: t("task:workflowMovePreviewModelUnknown"),
      detail: undefined,
      changeCount,
    };
  }

  let label: string;
  if (!before.known && preview.model.after_source === "profile") {
    label = t("task:workflowMovePreviewModelPlanned", { model: after.label });
  } else if (before.known && before.label && before.label !== after.label) {
    label = t("task:workflowMovePreviewModelChange", {
      before: before.label,
      after: after.label,
    });
  } else if (preview.model.after_source === "override") {
    label = t("task:workflowMovePreviewModelRetainedOverride", { model: after.label });
  } else {
    label = t("task:workflowMovePreviewModel", { model: after.label });
  }
  return { label, changeCount };
}

function applicabilityLabel(t: Translation, applicability: WorkflowMovePreviewApplicability) {
  switch (applicability) {
    case "planned":
      return t("task:workflowMovePreviewPlanned");
    case "skipped":
      return t("task:workflowMovePreviewSkipped");
    case "unchanged":
      return t("task:workflowMovePreviewUnchanged");
    default:
      return t(UNKNOWN_PREVIEW_LABEL_KEY);
  }
}

function sourceDispositionLabel(
  t: Translation,
  value: WorkflowMovePreviewResponse["source_disposition"],
): string {
  switch (value) {
    case "keep":
      return t("task:workflowMovePreviewSourceKeep");
    case "park":
      return t("task:workflowMovePreviewSourcePark");
    case "complete":
      return t("task:workflowMovePreviewSourceComplete");
    default:
      return t(UNKNOWN_PREVIEW_LABEL_KEY);
  }
}

function dispatchLabel(t: Translation, value: WorkflowMovePreviewResponse["dispatch"]): string {
  switch (value) {
    case "prompt":
      return t("task:workflowMovePreviewDispatchPrompt");
    case "no_prompt":
      return t("task:workflowMovePreviewDispatchNoPrompt");
    case "deferred":
      return t("task:workflowMovePreviewDispatchDeferred");
    case "no_session":
      return t("task:workflowMovePreviewDispatchNoSession");
    default:
      return t(UNKNOWN_PREVIEW_LABEL_KEY);
  }
}

function noticeLabel(t: Translation, code: string): string {
  const translationKey = PREVIEW_NOTICE_TRANSLATION_KEYS[code];
  return t(`task:${translationKey ?? "workflowMovePreviewNoticeUnavailable"}`);
}

function changeFieldLabel(t: Translation, key: string, providerLabel: string): string {
  const translationKey = PREVIEW_FIELD_TRANSLATION_KEYS[key];
  return translationKey
    ? t(`task:${translationKey}`)
    : providerLabel || t("task:workflowMovePreviewFieldOther");
}

function changeValueLabel(t: Translation, key: string, value: string | undefined): string {
  if (!value) return t(UNKNOWN_PREVIEW_LABEL_KEY);
  const translationKey = PREVIEW_VALUE_TRANSLATION_KEYS[key]?.[value];
  return translationKey ? t(`task:${translationKey}`) : value;
}

function displayValue(t: Translation, value: string | undefined, known = true): string {
  return known && value ? value : t(UNKNOWN_PREVIEW_LABEL_KEY);
}

function WorkflowMovePreviewDetails({
  preview,
  t,
}: {
  preview: WorkflowMovePreviewResponse;
  t: Translation;
}) {
  const recipient = preview.recipient;
  return (
    <div
      data-testid="workflow-move-preview-details"
      className="grid gap-1.5 text-left border-t border-border/60 pt-1.5 text-[11px] text-muted-foreground"
    >
      <div className="grid gap-0.5">
        <span className="font-medium text-foreground">{t("task:workflowMovePreviewSession")}</span>
        <span className="truncate" title={recipient?.session_name ?? undefined}>
          {displayValue(t, recipient?.session_name)}
        </span>
      </div>
      <div className="grid gap-0.5">
        <span className="font-medium text-foreground">{t("task:workflowMovePreviewProfile")}</span>
        <span className="truncate" title={recipient?.profile_name ?? undefined}>
          {displayValue(t, recipient?.profile_name)}
        </span>
      </div>
      <div className="grid gap-0.5">
        <span className="font-medium text-foreground">
          {t("task:workflowMovePreviewModelDetails")}
        </span>
        <span>
          {displayValue(t, preview.model.before.label, preview.model.before.known)}{" "}
          {t("task:workflowMovePreviewTo")}{" "}
          {displayValue(t, preview.model.after.label, preview.model.after.known)}
        </span>
      </div>
      {(preview.changes ?? []).length > 0 && (
        <div className="grid gap-1">
          <span className="font-medium text-foreground">
            {t("task:workflowMovePreviewChanges")}
          </span>
          {preview.changes?.map((change) => {
            const label = changeFieldLabel(t, change.key, change.label);
            return (
              <div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2" key={change.key}>
                <span className="truncate" title={label}>
                  {label}
                </span>
                <span className="text-right">
                  {changeValueLabel(t, change.key, change.before)} {t("task:workflowMovePreviewTo")}{" "}
                  {changeValueLabel(t, change.key, change.after)}
                  <span className="ml-1">({applicabilityLabel(t, change.applicability)})</span>
                </span>
              </div>
            );
          })}
        </div>
      )}
      {preview.context_reset && (
        <div>
          <span className="font-medium text-foreground">
            {t("task:workflowMovePreviewContext")}:{" "}
          </span>
          {t("task:workflowMovePreviewContextReset")} (
          {applicabilityLabel(t, preview.context_reset_state)})
        </div>
      )}
      <div>
        <span className="font-medium text-foreground">{t("task:workflowMovePreviewSource")}: </span>
        {sourceDispositionLabel(t, preview.source_disposition)}
      </div>
      <div>
        <span className="font-medium text-foreground">
          {t("task:workflowMovePreviewDispatch")}:{" "}
        </span>
        {dispatchLabel(t, preview.dispatch)}
      </div>
      {(preview.notices ?? []).length > 0 && (
        <div className="grid gap-0.5">
          <span className="font-medium text-foreground">{t("task:workflowMovePreviewNotes")}</span>
          {preview.notices?.map((notice, index) => (
            <span key={`${notice.code}-${index}`}>{noticeLabel(t, notice.code)}</span>
          ))}
        </div>
      )}
      <span>{t("task:workflowMovePreviewExecutionCheck")}</span>
    </div>
  );
}

type CompactWorkflowMovePreviewProps = Omit<WorkflowMovePreviewDisclosureProps, "className"> & {
  expanded: boolean;
};

export function CompactWorkflowMovePreview({
  state,
  isTouchSurface,
  expanded,
}: CompactWorkflowMovePreviewProps) {
  const { t } = useTranslation();
  if (state.status === "idle") return null;
  if (state.status === "loading") {
    return (
      <div
        role="status"
        data-testid="workflow-move-preview-loading"
        className="pl-4 text-left text-[11px] text-muted-foreground"
      >
        {t("task:workflowMovePreviewChecking")}
      </div>
    );
  }
  if (state.status === "error" || !state.preview) {
    return (
      <div
        role="status"
        data-testid="workflow-move-preview-error"
        className="flex min-w-0 items-center gap-1 pl-4 text-left text-[11px] text-muted-foreground"
      >
        <span className="min-w-0 truncate">{t("task:workflowMovePreviewUnavailable")}</span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={state.retry}
          className={cn(
            "shrink-0 cursor-pointer px-1.5 text-[11px]",
            isTouchSurface ? "min-h-11" : "h-7",
          )}
          data-testid="workflow-move-preview-retry"
        >
          <IconRefresh className="h-3 w-3" aria-hidden="true" />
          {t("task:workflowMovePreviewRetry")}
        </Button>
      </div>
    );
  }
  const model = modelSummaryParts(t, state.preview);
  const summary = [
    outcomeLabel(t, state.preview),
    model.label,
    ...(model.changeCount > 0
      ? [t("task:workflowMovePreviewAdditionalChanges", { count: model.changeCount })]
      : []),
  ].join(" · ");
  return (
    <div
      data-testid="workflow-move-preview"
      className="grid min-w-0 gap-2 pl-4 text-left text-[11px] text-muted-foreground"
    >
      <div role="status" aria-live="polite" className="truncate" title={summary}>
        {summary}
      </div>
      {expanded && <WorkflowMovePreviewDetails preview={state.preview} t={t} />}
    </div>
  );
}

export function WorkflowMovePreviewDisclosure({
  state,
  isTouchSurface,
  className,
}: WorkflowMovePreviewDisclosureProps) {
  const { t } = useTranslation();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const retryButtonClass = isTouchSurface ? "min-h-11" : "h-7";
  const detailsButtonClass = isTouchSurface ? "min-h-11 min-w-11 p-0" : "h-7 w-7 p-0";

  if (state.status === "idle") return null;
  if (state.status === "loading") {
    return (
      <div
        data-testid="workflow-move-preview-loading"
        role="status"
        aria-live="polite"
        className={cn(
          "grid w-full min-w-0 gap-0.5 border-t border-border/60 pt-2 text-center text-[11px] text-muted-foreground",
          className,
        )}
      >
        <span className="flex min-w-0 items-center justify-center gap-1 truncate">
          <IconLoader2 className="h-3 w-3 shrink-0 animate-spin" aria-hidden="true" />
          {t("task:workflowMovePreviewChecking")}
        </span>
        <span className="h-4" aria-hidden="true" />
      </div>
    );
  }
  if (state.status === "error" || !state.preview) {
    return (
      <div
        data-testid="workflow-move-preview-error"
        role="status"
        aria-live="polite"
        className={cn(
          "grid w-full min-w-0 gap-0.5 border-t border-border/60 pt-2 text-center text-[11px] text-muted-foreground",
          className,
        )}
      >
        <span className="truncate">{t("task:workflowMovePreviewUnavailable")}</span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn("justify-self-center px-1.5 text-[11px]", retryButtonClass)}
          onClick={state.retry}
          data-testid="workflow-move-preview-retry"
        >
          <IconRefresh className="h-3 w-3" aria-hidden="true" />
          {t("task:workflowMovePreviewRetry")}
        </Button>
      </div>
    );
  }

  const preview = state.preview;
  const model = modelSummaryParts(t, preview);
  const firstLine = outcomeLabel(t, preview);
  return (
    <div
      data-testid="workflow-move-preview"
      className={cn(
        "grid w-full min-w-0 gap-0.5 border-t border-border/60 pt-2 text-center text-[11px] text-muted-foreground",
        className,
      )}
    >
      <div role="status" aria-live="polite" className="min-w-0 truncate" title={firstLine}>
        {firstLine}
      </div>
      <div className="flex min-w-0 items-center justify-center gap-0">
        <span className="min-w-0 truncate" title={model.label}>
          {model.label}
          {model.changeCount > 0 && (
            <span className="ml-1 text-muted-foreground">
              {t("task:workflowMovePreviewAdditionalChanges", { count: model.changeCount })}
            </span>
          )}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className={cn("shrink-0 gap-1 text-[11px]", detailsButtonClass)}
          aria-expanded={detailsOpen}
          aria-label={t(
            detailsOpen
              ? "task:workflowMovePreviewHideDetails"
              : "task:workflowMovePreviewShowDetails",
          )}
          onClick={() => setDetailsOpen((open) => !open)}
          data-testid="workflow-move-preview-details-toggle"
        >
          <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
        </Button>
      </div>
      {detailsOpen && <WorkflowMovePreviewDetails preview={preview} t={t} />}
    </div>
  );
}
