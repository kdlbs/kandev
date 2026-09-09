"use client";

import { useTranslation } from "react-i18next";
import { IconChevronLeft, IconChevronRight, IconRobot } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import { AgentLogo } from "@/components/agent-logo";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import type {
  WorkflowProfileSessionEndPolicy,
  WorkflowProfileSessionStartPolicy,
  WorkflowSessionTarget,
  WorkflowStep,
} from "@/lib/types/http";
import {
  normalizeWorkflowProfileSessionEndPolicy,
  normalizeWorkflowProfileSessionStartPolicy,
} from "@/lib/types/http";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";

type StartOption = {
  value: WorkflowProfileSessionStartPolicy;
  labelKey: string;
  descriptionKey: string;
  shortLabelKey: string;
};

type EndOption = {
  value: WorkflowProfileSessionEndPolicy;
  labelKey: string;
  descriptionKey: string;
  shortLabelKey: string;
};

const START_OPTIONS: StartOption[] = [
  {
    value: "reuse",
    labelKey: "workflows:profileSessionStartReuse",
    descriptionKey: "workflows:profileSessionStartReuseDescription",
    shortLabelKey: "workflows:profileSessionStartReuseShort",
  },
  {
    value: "new",
    labelKey: "workflows:profileSessionStartNew",
    descriptionKey: "workflows:profileSessionStartNewDescription",
    shortLabelKey: "workflows:profileSessionStartNewShort",
  },
];

const END_OPTIONS: EndOption[] = [
  {
    value: "complete",
    labelKey: "workflows:profileSessionEndComplete",
    descriptionKey: "workflows:profileSessionEndCompleteDescription",
    shortLabelKey: "workflows:profileSessionEndCompleteShort",
  },
  {
    value: "park",
    labelKey: "workflows:profileSessionEndPark",
    descriptionKey: "workflows:profileSessionEndParkDescription",
    shortLabelKey: "workflows:profileSessionEndParkShort",
  },
];

const PROFILE_SESSION_LIFECYCLE_KEY = "workflows:profileSessionLifecycle";
const NO_PROFILE_OVERRIDE_KEY = "workflows:noProfileOverride";

export type SelectorView = "profiles" | "session";

export type WorkflowSessionSelectorSurfaceProps = {
  step: WorkflowStep;
  steps: WorkflowStep[];
  profiles: ReturnType<typeof useHealthyAgentProfiles>;
  readOnly: boolean;
  profileSelectionDisabled: boolean;
  view: SelectorView;
  setView: (view: SelectorView) => void;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onSelectProfile: (profileId: string) => void;
  onSelectTarget: (target: WorkflowSessionTarget) => void;
  onClose: () => void;
};

function startOption(value: unknown) {
  const normalized = normalizeWorkflowProfileSessionStartPolicy(value);
  return START_OPTIONS.find((option) => option.value === normalized) ?? START_OPTIONS[0];
}

function endOption(value: unknown) {
  const normalized = normalizeWorkflowProfileSessionEndPolicy(value);
  return END_OPTIONS.find((option) => option.value === normalized) ?? END_OPTIONS[0];
}

export function lifecycleSummary(step: WorkflowStep, translate: (key: string) => string): string {
  return (
    translate(startOption(step.profile_session_start_policy).shortLabelKey) +
    " · " +
    translate(endOption(step.profile_session_end_policy).shortLabelKey)
  );
}

function WorkflowSessionTargetGroup({
  step,
  steps,
  profiles,
  targetsDisabled,
  onSelectTarget,
}: Pick<WorkflowSessionSelectorSurfaceProps, "step" | "steps" | "profiles" | "onSelectTarget"> & {
  targetsDisabled: boolean;
}) {
  const { t } = useTranslation();
  const earlierProfileSteps = steps.filter(
    (candidate) =>
      candidate.id !== step.id &&
      candidate.position < step.position &&
      !!candidate.agent_profile_id &&
      !candidate.session_target,
  );
  const selectedTarget = step.session_target;

  return (
    <CommandGroup heading={t("workflows:workflowSessions")}>
      <CommandItem
        value={"initial " + t("workflows:initialAgentSession")}
        disabled={targetsDisabled}
        aria-selected={selectedTarget?.kind === "initial"}
        data-checked={selectedTarget?.kind === "initial"}
        data-testid={step.id + "-session-target-initial"}
        onSelect={() => onSelectTarget({ kind: "initial" })}
        className="min-h-11 cursor-pointer"
      >
        <IconRobot className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="min-w-0 truncate">
          <span className="block truncate">{t("workflows:initialAgentSession")}</span>
          <span className="block truncate text-xs text-muted-foreground">
            {t("workflows:initialAgentSessionDescription")}
          </span>
        </span>
      </CommandItem>
      {earlierProfileSteps.map((source) => {
        const profile = profiles.find((candidate) => candidate.id === source.agent_profile_id);
        const selected = selectedTarget?.kind === "step" && selectedTarget.step_id === source.id;
        return (
          <CommandItem
            key={source.id}
            value={source.name + " " + (profile?.label ?? source.agent_profile_id)}
            disabled={targetsDisabled}
            aria-selected={selected}
            data-checked={selected}
            data-testid={step.id + "-session-target-step-" + source.id}
            onSelect={() => onSelectTarget({ kind: "step", step_id: source.id })}
            className="min-h-11 cursor-pointer"
          >
            {profile ? (
              <AgentLogo agentName={profile.agent_name} className="shrink-0" />
            ) : (
              <IconRobot className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            )}
            <span className="truncate">
              {t("workflows:sessionTargetStepLabel", {
                stepName: source.name,
                profileName: profile?.label ?? t("workflows:profileUnavailable"),
              })}
            </span>
          </CommandItem>
        );
      })}
    </CommandGroup>
  );
}

function AgentProfileGroup({
  step,
  profiles,
  readOnly,
  onSelect,
}: Pick<WorkflowSessionSelectorSurfaceProps, "step" | "profiles" | "readOnly"> & {
  onSelect: (profileId: string) => void;
}) {
  const { t } = useTranslation();
  const selectedTarget = step.session_target;
  return (
    <CommandGroup heading={t("workflows:agentProfiles")}>
      <CommandItem
        value={"none " + t(NO_PROFILE_OVERRIDE_KEY)}
        disabled={readOnly}
        aria-selected={!step.agent_profile_id && !selectedTarget}
        data-checked={!step.agent_profile_id && !selectedTarget}
        data-testid={step.id + "-profile-option-none"}
        onSelect={() => onSelect("")}
        className="min-h-11 cursor-pointer"
      >
        <IconRobot className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="truncate">{t(NO_PROFILE_OVERRIDE_KEY)}</span>
      </CommandItem>
      {profiles.map((profile) => (
        <CommandItem
          key={profile.id}
          value={profile.id + " " + profile.label}
          disabled={readOnly}
          aria-selected={!selectedTarget && step.agent_profile_id === profile.id}
          data-checked={!selectedTarget && step.agent_profile_id === profile.id}
          data-testid={step.id + "-profile-option-" + profile.id}
          onSelect={() => onSelect(profile.id)}
          className="min-h-11 cursor-pointer"
        >
          <AgentLogo agentName={profile.agent_name} className="shrink-0" />
          <span className="truncate">{profile.label}</span>
        </CommandItem>
      ))}
    </CommandGroup>
  );
}

function ProfileOptionList({
  step,
  steps,
  profiles,
  readOnly,
  profileSelectionDisabled,
  onSelectTarget,
  onSelectProfile,
}: Pick<
  WorkflowSessionSelectorSurfaceProps,
  | "step"
  | "steps"
  | "profiles"
  | "readOnly"
  | "profileSelectionDisabled"
  | "onSelectTarget"
  | "onSelectProfile"
>) {
  const { t } = useTranslation();
  return (
    <Command shouldFilter>
      <CommandInput
        autoFocus
        placeholder={t("agents:searchDynamicCandidates")}
        aria-label={t("agents:searchDynamicCandidates")}
      />
      <CommandList
        className="max-h-none !overflow-visible overscroll-contain"
        onWheel={(event) => event.stopPropagation()}
      >
        <CommandEmpty>{t("agents:profileNotFound")}</CommandEmpty>
        <WorkflowSessionTargetGroup
          step={step}
          steps={steps}
          profiles={profiles}
          targetsDisabled={profileSelectionDisabled}
          onSelectTarget={onSelectTarget}
        />
        <AgentProfileGroup
          step={step}
          profiles={profiles}
          readOnly={readOnly || profileSelectionDisabled}
          onSelect={onSelectProfile}
        />
      </CommandList>
    </Command>
  );
}

function LifecycleOptionList({
  step,
  readOnly,
  onStartSelect,
  onEndSelect,
}: Pick<WorkflowSessionSelectorSurfaceProps, "step" | "readOnly"> & {
  onStartSelect: (policy: WorkflowProfileSessionStartPolicy) => void;
  onEndSelect: (policy: WorkflowProfileSessionEndPolicy) => void;
}) {
  const { t } = useTranslation();
  const selectedStart = normalizeWorkflowProfileSessionStartPolicy(
    step.profile_session_start_policy,
  );
  const selectedEnd = normalizeWorkflowProfileSessionEndPolicy(step.profile_session_end_policy);

  return (
    <div className="space-y-4 p-3">
      <p className="text-xs leading-relaxed text-muted-foreground">
        {t("workflows:profileSessionLifecycleIntro")}
      </p>
      <fieldset className="space-y-1">
        <legend className="px-1 pb-1 text-xs font-medium text-muted-foreground">
          {t("workflows:profileSessionStartHeading")}
        </legend>
        {START_OPTIONS.map((option) => (
          <button
            key={option.value}
            type="button"
            disabled={readOnly}
            className="flex min-h-11 w-full cursor-pointer flex-col items-start justify-center rounded-md border border-transparent px-3 py-2 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 data-[selected=true]:border-primary/40 data-[selected=true]:bg-primary/5"
            data-selected={selectedStart === option.value}
            aria-pressed={selectedStart === option.value}
            data-testid={step.id + "-profile-session-start-" + option.value}
            onClick={() => onStartSelect(option.value)}
          >
            <span className="font-medium">{t(option.labelKey)}</span>
            <span className="text-xs leading-relaxed text-muted-foreground">
              {t(option.descriptionKey)}
            </span>
          </button>
        ))}
      </fieldset>
      <fieldset className="space-y-1">
        <legend className="px-1 pb-1 text-xs font-medium text-muted-foreground">
          {t("workflows:profileSessionEndHeading")}
        </legend>
        {END_OPTIONS.map((option) => (
          <button
            key={option.value}
            type="button"
            disabled={readOnly}
            className="flex min-h-11 w-full cursor-pointer flex-col items-start justify-center rounded-md border border-transparent px-3 py-2 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 data-[selected=true]:border-primary/40 data-[selected=true]:bg-primary/5"
            data-selected={selectedEnd === option.value}
            aria-pressed={selectedEnd === option.value}
            data-testid={step.id + "-profile-session-end-" + option.value}
            onClick={() => onEndSelect(option.value)}
          >
            <span className="font-medium">{t(option.labelKey)}</span>
            <span className="text-xs leading-relaxed text-muted-foreground">
              {t(option.descriptionKey)}
            </span>
          </button>
        ))}
      </fieldset>
    </div>
  );
}

function LifecycleNavigation({
  step,
  readOnly,
  onOpen,
}: Pick<WorkflowSessionSelectorSurfaceProps, "step" | "readOnly"> & { onOpen: () => void }) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-md border border-border/70 px-3 py-2 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
      disabled={readOnly}
      data-testid={step.id + "-profile-session-lifecycle-select"}
      onClick={onOpen}
    >
      <span className="min-w-0 flex-1">
        <span className="block text-xs font-medium text-muted-foreground">
          {t(PROFILE_SESSION_LIFECYCLE_KEY)}
        </span>
        <span className="block truncate font-medium">{lifecycleSummary(step, t)}</span>
      </span>
      <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
  );
}

export function SelectorSurface({
  step,
  steps,
  profiles,
  readOnly,
  profileSelectionDisabled,
  view,
  setView,
  onUpdate,
  onSelectProfile,
  onSelectTarget,
}: WorkflowSessionSelectorSurfaceProps) {
  const { t } = useTranslation();
  if (view === "session") {
    return (
      <div className="min-w-0">
        <div className="flex items-center gap-2 border-b border-border px-3 py-2">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="h-9 w-9 shrink-0 cursor-pointer"
            aria-label={t("common:back")}
            onClick={() => setView("profiles")}
          >
            <IconChevronLeft className="h-4 w-4" aria-hidden="true" />
          </Button>
          <div className="min-w-0">
            <p className="font-medium">{t(PROFILE_SESSION_LIFECYCLE_KEY)}</p>
            <p className="truncate text-xs text-muted-foreground">
              {t("workflows:profileSessionLifecycleIntro")}
            </p>
          </div>
        </div>
        <div className="max-h-[min(70vh,32rem,calc(var(--radix-popover-content-available-height)-4rem))] overflow-y-auto overscroll-contain">
          <LifecycleOptionList
            step={step}
            readOnly={readOnly}
            onStartSelect={(policy) => onUpdate({ profile_session_start_policy: policy })}
            onEndSelect={(policy) => onUpdate({ profile_session_end_policy: policy })}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="min-w-0">
      <div className="border-t border-border p-2">
        <LifecycleNavigation step={step} readOnly={readOnly} onOpen={() => setView("session")} />
      </div>
      <div className="max-h-[min(70vh,32rem,calc(var(--radix-popover-content-available-height)-4rem))] overflow-y-auto overscroll-contain">
        <ProfileOptionList
          step={step}
          steps={steps}
          profiles={profiles}
          readOnly={readOnly}
          profileSelectionDisabled={profileSelectionDisabled}
          onSelectTarget={onSelectTarget}
          onSelectProfile={onSelectProfile}
        />
      </div>
    </div>
  );
}

export function MobileSelectorSurface({
  step,
  steps,
  profiles,
  readOnly,
  profileSelectionDisabled,
  view,
  setView,
  onUpdate,
  onSelectProfile,
  onSelectTarget,
  onClose,
}: WorkflowSessionSelectorSurfaceProps) {
  const { t } = useTranslation();
  if (view === "session") {
    return (
      <MobilePickerSheet
        open
        onOpenChange={(open) => !open && onClose()}
        title={t(PROFILE_SESSION_LIFECYCLE_KEY)}
        description={t("workflows:profileSessionLifecycleIntro")}
        contentTestId={step.id + "-profile-session-lifecycle-content"}
        fixedContent={
          <div className="border-b border-border px-3 py-2">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-9 cursor-pointer"
              aria-label={t("common:back")}
              onClick={() => setView("profiles")}
            >
              <IconChevronLeft className="mr-1 h-4 w-4" aria-hidden="true" />
              {t("common:back")}
            </Button>
          </div>
        }
      >
        <LifecycleOptionList
          step={step}
          readOnly={readOnly}
          onStartSelect={(policy) => onUpdate({ profile_session_start_policy: policy })}
          onEndSelect={(policy) => onUpdate({ profile_session_end_policy: policy })}
        />
      </MobilePickerSheet>
    );
  }

  return (
    <MobilePickerSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("workflows:agentProfile")}
      description={t("workflows:sessionTargetAndProfileDescription")}
      contentTestId={step.id + "-profile-picker-content"}
      fixedContent={
        <div className="border-b border-border p-2">
          <LifecycleNavigation step={step} readOnly={readOnly} onOpen={() => setView("session")} />
        </div>
      }
    >
      <ProfileOptionList
        step={step}
        steps={steps}
        profiles={profiles}
        readOnly={readOnly}
        profileSelectionDisabled={profileSelectionDisabled}
        onSelectTarget={onSelectTarget}
        onSelectProfile={onSelectProfile}
      />
    </MobilePickerSheet>
  );
}
