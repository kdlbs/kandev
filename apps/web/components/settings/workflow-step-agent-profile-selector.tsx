"use client";

import {
  forwardRef,
  useEffect,
  useRef,
  useState,
  type ComponentPropsWithoutRef,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconRobot, IconSelector } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { AgentLogo } from "@/components/agent-logo";
import type { WorkflowSessionTarget, WorkflowStep } from "@/lib/types/http";
import {
  normalizeWorkflowProfileSessionEndPolicy,
  normalizeWorkflowProfileSessionStartPolicy,
} from "@/lib/types/http";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { settingsControlClassName } from "./settings-control";
import { HelpTip, hasOnEnterAction } from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";
import {
  MobileSelectorSurface,
  SelectorSurface,
  lifecycleSummary,
  type SelectorView,
} from "./workflow-session-target-surfaces";
import {
  getWorkflowSessionTargetIssue,
  type WorkflowSessionTargetIssue,
} from "./workflow-session-target-validation";

type WorkflowStepAgentProfileSelectorProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps?: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRestoreSource?: () => void;
  readOnly: boolean;
};

function targetStepFor(step: WorkflowStep, steps: WorkflowStep[]): WorkflowStep | undefined {
  const target = step.session_target;
  if (!target || target.kind !== "step") return undefined;
  return steps.find((candidate) => candidate.id === target.step_id);
}

function targetLabel(
  step: WorkflowStep,
  steps: WorkflowStep[],
  profiles: ReturnType<typeof useHealthyAgentProfiles>,
  translate: (key: string, options?: Record<string, unknown>) => string,
): string {
  const target = step.session_target;
  if (!target) return translate("workflows:noProfileOverride");
  if (target.kind === "initial") return translate("workflows:initialAgentSession");
  const source = targetStepFor(step, steps);
  const profile = source
    ? profiles.find((candidate) => candidate.id === source.agent_profile_id)
    : undefined;
  return translate("workflows:sessionTargetStepLabel", {
    stepName: source?.name ?? translate("workflows:sessionTargetMissing"),
    profileName: profile?.label ?? translate("workflows:profileUnavailable"),
  });
}

function sessionTargetIssueLabel(
  issue: WorkflowSessionTargetIssue,
  translate: (key: string) => string,
): string {
  switch (issue.kind) {
    case "missing-source":
      return translate("workflows:sessionTargetSourceMissing");
    case "not-earlier":
      return translate("workflows:sessionTargetSourceMustBeEarlier");
    case "missing-profile":
      return translate("workflows:sessionTargetSourceNeedsProfile");
    case "indirect-source":
      return translate("workflows:sessionTargetSourceMustBeDirect");
  }
}

function SessionTargetRepair({
  issue,
  canRestoreSource,
  readOnly,
  onChooseAnother,
  onClear,
  onRestoreSource,
}: {
  issue: WorkflowSessionTargetIssue;
  canRestoreSource: boolean;
  readOnly: boolean;
  onChooseAnother: () => void;
  onClear: () => void;
  onRestoreSource?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="w-full space-y-2 rounded-md border border-destructive/40 bg-destructive/5 p-3"
      data-testid="workflow-session-target-repair"
    >
      <p className="flex items-start gap-2 text-xs text-destructive">
        <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <span>
          <span className="block font-medium">{t("workflows:sessionTargetRepairTitle")}</span>
          <span>{sessionTargetIssueLabel(issue, t)}</span>
        </span>
      </p>
      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap">
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 cursor-pointer"
          onClick={onChooseAnother}
          data-testid="workflow-session-target-choose-another"
        >
          {t("workflows:sessionTargetChooseAnother")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-11 cursor-pointer"
          disabled={readOnly}
          onClick={onClear}
          data-testid="workflow-session-target-clear"
        >
          {t("workflows:sessionTargetUseNoProfile")}
        </Button>
        {canRestoreSource && onRestoreSource && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="min-h-11 cursor-pointer"
            disabled={readOnly}
            onClick={onRestoreSource}
            data-testid="workflow-session-target-restore-source"
          >
            {t("workflows:sessionTargetUndoSourceEdit")}
          </Button>
        )}
      </div>
    </div>
  );
}

type SelectorTriggerProps = {
  selectedProfile: ReturnType<typeof useHealthyAgentProfiles>[number] | undefined;
  selectionLabel: string;
  summary: string;
  open: boolean;
  disabled: boolean;
  dirty: boolean;
  onOpen?: () => void;
} & ComponentPropsWithoutRef<"button">;

const SelectorTrigger = forwardRef<HTMLButtonElement, SelectorTriggerProps>(
  function SelectorTrigger(
    {
      selectedProfile,
      selectionLabel,
      summary,
      open,
      disabled,
      dirty,
      onOpen,
      onClick,
      ...triggerProps
    },
    ref,
  ) {
    const { t } = useTranslation();
    return (
      <Button
        ref={ref}
        type="button"
        variant="outline"
        role="combobox"
        aria-expanded={open}
        aria-label={t("workflows:agentProfile")}
        disabled={disabled}
        data-testid="step-agent-profile-select"
        data-settings-dirty={dirty}
        onClick={(event) => {
          onClick?.(event);
          onOpen?.();
        }}
        {...triggerProps}
        className={settingsControlClassName(
          "h-auto w-full min-w-0 cursor-pointer justify-between px-2 text-left sm:w-[280px]",
        )}
      >
        <span className="flex min-w-0 items-center gap-2">
          {selectedProfile ? (
            <AgentLogo agentName={selectedProfile.agent_name} className="shrink-0" />
          ) : (
            <IconRobot className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
          )}
          <span className="min-w-0 truncate">
            <span className="block truncate">{selectionLabel}</span>
            <span className="block truncate text-[0.65rem] text-muted-foreground">{summary}</span>
          </span>
        </span>
        <IconSelector className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
      </Button>
    );
  },
);

type SelectorPopupProps = {
  isMobile: boolean;
  trigger: ReactNode;
  surface: ReactNode;
  mobileSurface: ReactNode;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
};

function SelectorPopup({
  isMobile,
  trigger,
  surface,
  mobileSurface,
  open,
  onOpenChange,
  triggerRef,
}: SelectorPopupProps) {
  if (isMobile) {
    return (
      <>
        {trigger}
        {open ? mobileSurface : null}
      </>
    );
  }
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        className="max-h-[var(--radix-popover-content-available-height)] w-[min(24rem,calc(100vw-2rem))] overflow-hidden p-0"
        align="start"
        onWheel={(event) => event.stopPropagation()}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus({ preventScroll: true });
        }}
      >
        {surface}
      </PopoverContent>
    </Popover>
  );
}

type SelectorContentProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  steps: WorkflowStep[];
  profiles: ReturnType<typeof useHealthyAgentProfiles>;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onRestoreSource?: () => void;
  readOnly: boolean;
  isMobile: boolean;
  open: boolean;
  view: SelectorView;
  setView: (view: SelectorView) => void;
  handleOpenChange: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
};

type SelectorChoicePopupProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  profiles: ReturnType<typeof useHealthyAgentProfiles>;
  selectedProfile: ReturnType<typeof useHealthyAgentProfiles>[number] | undefined;
  readOnly: boolean;
  profileSelectionDisabled: boolean;
  selectionLabel: string;
  summary: string;
  dirty: boolean;
  isMobile: boolean;
  open: boolean;
  view: SelectorView;
  setView: (view: SelectorView) => void;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onSelectProfile: (profileId: string) => void;
  onSelectTarget: (target: WorkflowSessionTarget) => void;
  handleOpenChange: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
};

function SelectorChoicePopup({
  step,
  steps,
  profiles,
  selectedProfile,
  readOnly,
  profileSelectionDisabled,
  selectionLabel,
  summary,
  dirty,
  isMobile,
  open,
  view,
  setView,
  onUpdate,
  onSelectProfile,
  onSelectTarget,
  handleOpenChange,
  triggerRef,
}: SelectorChoicePopupProps) {
  const trigger = (
    <SelectorTrigger
      selectedProfile={selectedProfile}
      selectionLabel={selectionLabel}
      summary={summary}
      open={open}
      disabled={false}
      dirty={dirty}
      ref={triggerRef}
      onOpen={isMobile ? () => handleOpenChange(true) : undefined}
    />
  );
  const surface = (
    <SelectorSurface
      step={step}
      steps={steps}
      profiles={profiles}
      readOnly={readOnly}
      profileSelectionDisabled={profileSelectionDisabled}
      view={view}
      setView={setView}
      onUpdate={onUpdate}
      onSelectProfile={onSelectProfile}
      onSelectTarget={onSelectTarget}
      onClose={() => handleOpenChange(false)}
    />
  );
  const mobileSurface = (
    <MobileSelectorSurface
      step={step}
      steps={steps}
      profiles={profiles}
      readOnly={readOnly}
      profileSelectionDisabled={profileSelectionDisabled}
      view={view}
      setView={setView}
      onUpdate={onUpdate}
      onSelectProfile={onSelectProfile}
      onSelectTarget={onSelectTarget}
      onClose={() => handleOpenChange(false)}
    />
  );
  return (
    <SelectorPopup
      isMobile={isMobile}
      trigger={trigger}
      surface={surface}
      mobileSurface={mobileSurface}
      open={open}
      onOpenChange={handleOpenChange}
      triggerRef={triggerRef}
    />
  );
}

function SelectorContent({
  step,
  savedStep,
  steps,
  profiles,
  onUpdate,
  onRestoreSource,
  readOnly,
  isMobile,
  open,
  view,
  setView,
  handleOpenChange,
  triggerRef,
}: SelectorContentProps) {
  const { t } = useTranslation();
  const targetStep = targetStepFor(step, steps);
  const targetIssue = getWorkflowSessionTargetIssue(step, steps);
  const selectedProfile = profiles.find(
    (profile) => profile.id === (targetStep?.agent_profile_id ?? step.agent_profile_id),
  );
  const hasConditionalSessionConfig = hasOnEnterAction(step, "configure_session");
  const dirty = isWorkflowStepValueDirty(step, savedStep, (item) =>
    JSON.stringify({
      agent_profile_id: item.agent_profile_id ?? "",
      profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
        item.profile_session_start_policy,
      ),
      profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
        item.profile_session_end_policy,
      ),
      session_target: item.session_target ?? null,
    }),
  );

  const selectProfile = (profileId: string) => {
    if (readOnly || hasConditionalSessionConfig) return;
    onUpdate({
      agent_profile_id: profileId,
      ...(step.session_target ? { session_target: null } : {}),
    });
    handleOpenChange(false);
  };

  const selectTarget = (target: WorkflowSessionTarget) => {
    if (readOnly || hasConditionalSessionConfig) return;
    onUpdate({ session_target: target, agent_profile_id: "" });
    handleOpenChange(false);
  };

  const selectionLabel = step.session_target
    ? targetLabel(step, steps, profiles, t)
    : (selectedProfile?.label ?? t("workflows:noProfileOverride"));
  const summary = lifecycleSummary(step, t);

  return (
    <div className="flex w-full min-w-0 flex-wrap items-center gap-2 sm:w-auto">
      <SelectorChoicePopup
        step={step}
        steps={steps}
        profiles={profiles}
        selectedProfile={selectedProfile}
        readOnly={readOnly}
        profileSelectionDisabled={readOnly || hasConditionalSessionConfig}
        selectionLabel={selectionLabel}
        summary={summary}
        dirty={dirty}
        isMobile={isMobile}
        open={open}
        view={view}
        setView={setView}
        onUpdate={onUpdate}
        onSelectProfile={selectProfile}
        onSelectTarget={selectTarget}
        handleOpenChange={handleOpenChange}
        triggerRef={triggerRef}
      />
      {targetIssue && (
        <SessionTargetRepair
          issue={targetIssue}
          canRestoreSource={Boolean(targetStep)}
          readOnly={readOnly}
          onChooseAnother={() => handleOpenChange(true)}
          onClear={() => onUpdate({ session_target: null, agent_profile_id: "" })}
          onRestoreSource={onRestoreSource}
        />
      )}
      <HelpTip
        testId={step.id + "-agent-profile-help"}
        text={
          hasConditionalSessionConfig
            ? t("workflows:removeConditionalSessionConfigBeforeProfile")
            : t("workflows:overrideAgentProfileHelp")
        }
      />
    </div>
  );
}

export function WorkflowStepAgentProfileSelector({
  step,
  savedStep,
  steps = [],
  onUpdate,
  onRestoreSource,
  readOnly,
}: WorkflowStepAgentProfileSelectorProps) {
  const { isMobile } = useResponsiveBreakpoint();
  const targetStep = targetStepFor(step, steps);
  const profiles = useHealthyAgentProfiles(step.agent_profile_id || targetStep?.agent_profile_id);
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<SelectorView>("profiles");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const wasOpenRef = useRef(false);

  useEffect(() => {
    if (wasOpenRef.current && !open) {
      requestAnimationFrame(() => triggerRef.current?.focus({ preventScroll: true }));
    }
    wasOpenRef.current = open;
  }, [open]);

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (nextOpen) setView("profiles");
  };

  return (
    <SelectorContent
      step={step}
      savedStep={savedStep}
      steps={steps}
      profiles={profiles}
      onUpdate={onUpdate}
      onRestoreSource={onRestoreSource}
      readOnly={readOnly}
      isMobile={isMobile}
      open={open}
      view={view}
      setView={setView}
      handleOpenChange={handleOpenChange}
      triggerRef={triggerRef}
    />
  );
}
