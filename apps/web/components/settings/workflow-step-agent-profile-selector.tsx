"use client";

import { forwardRef, useEffect, useRef, useState, type ComponentPropsWithoutRef } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronLeft, IconChevronRight, IconRobot, IconSelector } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@kandev/ui/command";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { AgentLogo } from "@/components/agent-logo";
import type {
  WorkflowProfileSessionEndPolicy,
  WorkflowProfileSessionStartPolicy,
  WorkflowStep,
} from "@/lib/types/http";
import {
  normalizeWorkflowProfileSessionEndPolicy,
  normalizeWorkflowProfileSessionStartPolicy,
} from "@/lib/types/http";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useHealthyAgentProfiles } from "@/hooks/domains/settings/use-healthy-agent-profiles";
import { settingsControlClassName } from "./settings-control";
import { HelpTip, hasOnEnterAction } from "./workflow-pipeline-editor-helpers";
import { isWorkflowStepValueDirty } from "./workflow-dirty-state";

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

type SelectorView = "profiles" | "session";

type WorkflowStepAgentProfileSelectorProps = {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  readOnly: boolean;
};

function startOption(value: unknown) {
  const normalized = normalizeWorkflowProfileSessionStartPolicy(value);
  return START_OPTIONS.find((option) => option.value === normalized) ?? START_OPTIONS[0];
}

function endOption(value: unknown) {
  const normalized = normalizeWorkflowProfileSessionEndPolicy(value);
  return END_OPTIONS.find((option) => option.value === normalized) ?? END_OPTIONS[0];
}

function lifecycleSummary(step: WorkflowStep, translate: (key: string) => string): string {
  return `${translate(startOption(step.profile_session_start_policy).shortLabelKey)} · ${translate(endOption(step.profile_session_end_policy).shortLabelKey)}`;
}

function ProfileOptionList({
  step,
  profiles,
  readOnly,
  onSelect,
}: {
  step: WorkflowStep;
  profiles: ReturnType<typeof useHealthyAgentProfiles>;
  readOnly: boolean;
  onSelect: (profileId: string) => void;
}) {
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
        <CommandGroup>
          <CommandItem
            value={`none ${t("workflows:noProfileOverride")}`}
            disabled={readOnly}
            aria-selected={!step.agent_profile_id}
            data-checked={!step.agent_profile_id}
            data-testid={`${step.id}-profile-option-none`}
            onSelect={() => onSelect("")}
            className="min-h-11 cursor-pointer"
          >
            <IconRobot className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
            <span className="truncate">{t("workflows:noProfileOverride")}</span>
          </CommandItem>
          {profiles.map((profile) => (
            <CommandItem
              key={profile.id}
              value={`${profile.id} ${profile.label}`}
              disabled={readOnly}
              aria-selected={step.agent_profile_id === profile.id}
              data-checked={step.agent_profile_id === profile.id}
              data-testid={`${step.id}-profile-option-${profile.id}`}
              onSelect={() => onSelect(profile.id)}
              className="min-h-11 cursor-pointer"
            >
              <AgentLogo agentName={profile.agent_name} className="shrink-0" />
              <span className="truncate">{profile.label}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </Command>
  );
}

function LifecycleOptionList({
  step,
  readOnly,
  onStartSelect,
  onEndSelect,
}: {
  step: WorkflowStep;
  readOnly: boolean;
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
            data-testid={`${step.id}-profile-session-start-${option.value}`}
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
            data-testid={`${step.id}-profile-session-end-${option.value}`}
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
}: {
  step: WorkflowStep;
  readOnly: boolean;
  onOpen: () => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      className="flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-md border border-border/70 px-3 py-2 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
      disabled={readOnly}
      data-testid={`${step.id}-profile-session-lifecycle-select`}
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

function SelectorSurface({
  step,
  profiles,
  readOnly,
  view,
  setView,
  onUpdate,
  onClose,
  mobile,
}: {
  step: WorkflowStep;
  profiles: ReturnType<typeof useHealthyAgentProfiles>;
  readOnly: boolean;
  view: SelectorView;
  setView: (view: SelectorView) => void;
  onUpdate: (updates: Partial<WorkflowStep>) => void;
  onClose: () => void;
  mobile: boolean;
}) {
  const { t } = useTranslation();
  const hasConditionalSessionConfig = hasOnEnterAction(step, "configure_session");
  const selectProfile = (profileId: string) => {
    if (readOnly || hasConditionalSessionConfig) return;
    onUpdate({ agent_profile_id: profileId });
    onClose();
  };

  if (view === "session") {
    return (
      <div className="max-h-[min(70vh,32rem)] min-w-0 overflow-y-auto overscroll-contain">
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
            {mobile ? (
              <DrawerTitle>{t(PROFILE_SESSION_LIFECYCLE_KEY)}</DrawerTitle>
            ) : (
              <p className="font-medium">{t(PROFILE_SESSION_LIFECYCLE_KEY)}</p>
            )}
            <p className="truncate text-xs text-muted-foreground">
              {t("workflows:profileSessionLifecycleIntro")}
            </p>
          </div>
        </div>
        <LifecycleOptionList
          step={step}
          readOnly={readOnly}
          onStartSelect={(profileSessionStartPolicy) =>
            onUpdate({ profile_session_start_policy: profileSessionStartPolicy })
          }
          onEndSelect={(profileSessionEndPolicy) =>
            onUpdate({ profile_session_end_policy: profileSessionEndPolicy })
          }
        />
      </div>
    );
  }

  return (
    <div className="max-h-[min(70vh,32rem)] min-w-0 overflow-y-auto overscroll-contain">
      {mobile ? (
        <DrawerHeader className="border-b border-border px-4 pb-3 pt-2 text-left">
          <DrawerTitle>{t("workflows:agentProfile")}</DrawerTitle>
          <DrawerDescription>{t(PROFILE_SESSION_LIFECYCLE_KEY)}</DrawerDescription>
        </DrawerHeader>
      ) : null}
      <ProfileOptionList
        step={step}
        profiles={profiles}
        readOnly={readOnly || hasConditionalSessionConfig}
        onSelect={selectProfile}
      />
      <div className="border-t border-border p-2">
        <LifecycleNavigation step={step} readOnly={readOnly} onOpen={() => setView("session")} />
      </div>
    </div>
  );
}

type SelectorTriggerProps = {
  selectedProfile: ReturnType<typeof useHealthyAgentProfiles>[number] | undefined;
  summary: string;
  open: boolean;
  disabled: boolean;
  dirty: boolean;
  onOpen?: () => void;
} & ComponentPropsWithoutRef<"button">;

const SelectorTrigger = forwardRef<HTMLButtonElement, SelectorTriggerProps>(
  function SelectorTrigger(
    { selectedProfile, summary, open, disabled, dirty, onOpen, onClick, ...triggerProps },
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
            <span className="block truncate">
              {selectedProfile?.label ?? t("workflows:noProfileOverride")}
            </span>
            <span className="block truncate text-[0.65rem] text-muted-foreground">{summary}</span>
          </span>
        </span>
        <IconSelector className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
      </Button>
    );
  },
);

export function WorkflowStepAgentProfileSelector({
  step,
  savedStep,
  onUpdate,
  readOnly,
}: WorkflowStepAgentProfileSelectorProps) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const profiles = useHealthyAgentProfiles(step.agent_profile_id);
  const [open, setOpen] = useState(false);
  const [view, setView] = useState<SelectorView>("profiles");
  const triggerRef = useRef<HTMLButtonElement>(null);
  const wasOpenRef = useRef(false);
  const selectedProfile = profiles.find((profile) => profile.id === step.agent_profile_id);
  const hasConditionalSessionConfig = hasOnEnterAction(step, "configure_session");
  const disabled = readOnly;
  const dirty = isWorkflowStepValueDirty(step, savedStep, (item) =>
    JSON.stringify({
      agent_profile_id: item.agent_profile_id ?? "",
      profile_session_start_policy: normalizeWorkflowProfileSessionStartPolicy(
        item.profile_session_start_policy,
      ),
      profile_session_end_policy: normalizeWorkflowProfileSessionEndPolicy(
        item.profile_session_end_policy,
      ),
    }),
  );

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

  const trigger = (
    <SelectorTrigger
      selectedProfile={selectedProfile}
      summary={lifecycleSummary(step, t)}
      open={open}
      disabled={disabled}
      dirty={dirty}
      ref={triggerRef}
      onOpen={isMobile ? () => handleOpenChange(true) : undefined}
    />
  );

  const surface = (
    <SelectorSurface
      step={step}
      profiles={profiles}
      readOnly={readOnly}
      view={view}
      setView={setView}
      onUpdate={onUpdate}
      onClose={() => setOpen(false)}
      mobile={isMobile}
    />
  );

  return (
    <div className="flex w-full min-w-0 flex-wrap items-center gap-2 sm:w-auto">
      {isMobile ? (
        <Drawer open={open} onOpenChange={handleOpenChange} shouldScaleBackground={false}>
          {trigger}
          <DrawerContent className="max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] overflow-hidden pb-[max(0.75rem,env(safe-area-inset-bottom))]">
            {surface}
          </DrawerContent>
        </Drawer>
      ) : (
        <Popover open={open} onOpenChange={handleOpenChange}>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent
            className="w-[min(24rem,calc(100vw-2rem))] overflow-hidden p-0"
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
      )}
      <HelpTip
        testId={`${step.id}-agent-profile-help`}
        text={
          hasConditionalSessionConfig
            ? t("workflows:removeConditionalSessionConfigBeforeProfile")
            : t("workflows:overrideAgentProfileHelp")
        }
      />
    </div>
  );
}
