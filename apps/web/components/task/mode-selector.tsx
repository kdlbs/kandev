"use client";

import {
  forwardRef,
  memo,
  type ComponentPropsWithoutRef,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { IconAlertTriangle, IconCheck, IconChevronDown, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { usableConfigOptions } from "@/components/model-config-selector";
import { useAppStore } from "@/components/state-provider";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import { MobilePillButton } from "@/components/task/mobile/mobile-pill-button";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAvailableAgents } from "@/hooks/domains/settings/use-available-agents";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { setSessionMode } from "@/lib/api/domains/session-api";
import { usePickerConfigurationFeedback } from "@/components/task/session-configuration-feedback";
import { cn } from "@/lib/utils";
import { prioritizeSelectedOption } from "@/lib/utils/selector-options";
import type { Agent, AgentProfile, AvailableAgent } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

const THINKING_LEVEL_LABEL = "agents:thinkingLevel";

type ModeOption = {
  id: string;
  name: string;
  description?: string;
};

type ModeSelectorState = {
  thinking?: boolean;
  currentModeId: string;
  availableModes: ModeOption[];
  requestedModeId?: string;
};

type ModeSelectorProps = {
  sessionId: string | null;
  triggerClassName?: string;
};

function translateModeLabel(t: ReturnType<typeof useTranslation>["t"], thinking?: boolean) {
  return t(thinking ? THINKING_LEVEL_LABEL : "task:agentPermissionMode");
}

function resolveSnapshotMode(snapshot: unknown): string | null {
  if (!snapshot || typeof snapshot !== "object") return null;
  const mode = (snapshot as Record<string, unknown>).mode;
  return typeof mode === "string" && mode ? mode : null;
}

function resolveProfileMode(profileId: string | null | undefined, agents: Agent[]): string | null {
  if (!profileId) return null;
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (profile?.mode) return profile.mode;
  }
  return null;
}

function resolveStaticModes(
  agents: Agent[],
  profileId: string | null | undefined,
  availableAgents: AvailableAgent[],
): ModeOption[] {
  if (!profileId) return [];
  for (const agent of agents) {
    const profile = agent.profiles.find((p: AgentProfile) => p.id === profileId);
    if (!profile) continue;
    const available = availableAgents.find((a: AvailableAgent) => a.name === agent.name);
    return available?.model_config?.available_modes ?? [];
  }
  return [];
}

function formatModeName(modeId: string): string {
  return modeId
    .split(/[-_\s]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

// withRequestedMode carries the requested mode onto the resolved state only
// when it differs from what the session is actually in.
function withRequestedMode(
  state: ModeSelectorState | undefined,
  requestedModeId: string | undefined,
): ModeSelectorState | undefined {
  if (!state || !requestedModeId || requestedModeId === state.currentModeId) return state;
  return { ...state, requestedModeId };
}

function buildModeState(
  currentModeId: string | null,
  liveModes: ModeOption[] | undefined,
  staticModes: ModeOption[],
): ModeSelectorState | undefined {
  const availableModes = liveModes?.length ? liveModes : staticModes;
  if (!currentModeId) {
    return availableModes.length > 0 ? { currentModeId: "", availableModes } : undefined;
  }
  if (availableModes.length === 0) {
    if (currentModeId === "default") return undefined;
    return {
      currentModeId,
      availableModes: [{ id: currentModeId, name: formatModeName(currentModeId) }],
    };
  }
  if (availableModes.length <= 1) return undefined;
  if (availableModes.some((m) => m.id === currentModeId)) {
    return {
      currentModeId,
      availableModes: prioritizeSelectedOption(availableModes, currentModeId, (mode) => mode.id),
    };
  }
  return {
    currentModeId,
    availableModes: [{ id: currentModeId, name: formatModeName(currentModeId) }, ...availableModes],
  };
}

function useModeSelectorState(sessionId: string | null) {
  useSettingsData(true);

  const liveModeState = useAppStore((state) =>
    sessionId ? state.sessionMode.bySessionId[sessionId] : undefined,
  );
  const configOptions = useAppStore((state) =>
    sessionId ? state.sessionModels.bySessionId[sessionId]?.configOptions : undefined,
  );
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const taskSessions = useAppStore((state) => state.taskSessions.items);
  const { items: availableAgents } = useAvailableAgents();

  const session = sessionId ? (taskSessions[sessionId] ?? null) : null;
  const isPi = session?.agent_profile_snapshot?.agent_name === "pi-acp";
  const snapshotMode = resolveSnapshotMode(session?.agent_profile_snapshot);
  const profileMode = useMemo(
    () => resolveProfileMode(session?.agent_profile_id, settingsAgents as Agent[]),
    [session?.agent_profile_id, settingsAgents],
  );
  const staticModes = useMemo(
    () => resolveStaticModes(settingsAgents as Agent[], session?.agent_profile_id, availableAgents),
    [availableAgents, session?.agent_profile_id, settingsAgents],
  );

  const hasThinkingConfig =
    isPi &&
    usableConfigOptions(configOptions).some((option) => option.category === "thought_level");
  return useMemo(() => {
    if (hasThinkingConfig) return undefined;
    const resolved = withRequestedMode(
      buildModeState(
        liveModeState ? liveModeState.currentModeId : snapshotMode || profileMode,
        liveModeState?.availableModes,
        staticModes,
      ),
      liveModeState?.requestedModeId,
    );
    return resolved ? { ...resolved, thinking: isPi } : undefined;
  }, [liveModeState, profileMode, snapshotMode, staticModes, isPi, hasThinkingConfig]);
}

function ModeMismatchWarning({
  requestedName,
  displayName,
  unconfirmed,
}: {
  requestedName: string;
  displayName: string;
  unconfirmed: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="mx-4 mb-2 flex items-start gap-2 rounded-md bg-amber-500/10 px-3 py-2 text-sm text-foreground"
      data-testid="session-mode-mismatch-warning"
      role="status"
    >
      <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" aria-hidden />
      <span>
        {unconfirmed
          ? t("task:sessionModeUnconfirmed", { requested: requestedName })
          : t("task:sessionModeNotApplied", { requested: requestedName, effective: displayName })}
      </span>
    </div>
  );
}

function MobileModeOption({
  mode,
  selected,
  onSelect,
}: {
  mode: ModeOption;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      data-testid={`session-mode-option-${mode.id}`}
      aria-label={mode.name}
      aria-pressed={selected}
      className={cn(
        "relative flex min-h-11 w-full cursor-pointer items-center rounded-md border px-3 py-2 pr-10 text-left",
        selected ? "border-primary/50 bg-card font-medium" : "border-transparent hover:bg-muted/60",
      )}
      onClick={onSelect}
    >
      <span className="min-w-0 flex-1">
        <span className="block">{mode.name}</span>
        {mode.description && (
          <span className="block text-xs text-muted-foreground">{mode.description}</span>
        )}
      </span>
      {selected && <IconCheck className="absolute right-3 h-4 w-4" aria-hidden />}
    </button>
  );
}

function MobileModeOptions({
  modeState,
  onModeChange,
  onClose,
}: {
  modeState: ModeSelectorState;
  onModeChange: (modeId: string) => Promise<void>;
  onClose: () => void;
}) {
  return (
    <div className="space-y-1">
      {modeState.availableModes.map((mode) => (
        <MobileModeOption
          key={mode.id}
          mode={mode}
          selected={mode.id === modeState.currentModeId}
          onSelect={() => {
            void onModeChange(mode.id);
            onClose();
          }}
        />
      ))}
    </div>
  );
}

function MobileModeSelector({
  modeState,
  displayName,
  requestedName,
  triggerClassName,
  onModeChange,
}: {
  modeState: ModeSelectorState;
  displayName: string;
  requestedName?: string;
  triggerClassName?: string;
  onModeChange: (modeId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);

  return (
    <>
      <MobilePillButton
        ref={triggerRef}
        icon={
          requestedName ? (
            <IconAlertTriangle className="h-3 w-3 shrink-0 text-amber-500" aria-hidden />
          ) : undefined
        }
        label={displayName}
        ariaLabel={displayName}
        isOpen={open}
        onClick={() => setOpen(true)}
        data-testid="session-mode-selector"
        className={cn("h-11 min-h-11 min-w-11", triggerClassName)}
      />
      <MobilePickerSheet
        open={open}
        onOpenChange={setOpen}
        title={t(modeState.thinking ? THINKING_LEVEL_LABEL : "task:agentPermissionMode")}
        description={t(modeState.thinking ? THINKING_LEVEL_LABEL : "task:availableModes")}
        contentTestId="session-mode-picker-scroll"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus({ preventScroll: true });
        }}
        fixedContent={
          requestedName ? (
            <ModeMismatchWarning
              requestedName={requestedName}
              displayName={displayName}
              unconfirmed={!modeState.currentModeId}
            />
          ) : undefined
        }
        headerAction={
          <Button
            variant="ghost"
            size="icon"
            className="h-12 w-12 shrink-0"
            aria-label={t("common:close")}
            data-testid="session-mode-mobile-picker-close"
            onClick={() => setOpen(false)}
          >
            <IconX className="h-4 w-4" aria-hidden />
          </Button>
        }
      >
        <MobileModeOptions
          modeState={modeState}
          onModeChange={onModeChange}
          onClose={() => setOpen(false)}
        />
      </MobilePickerSheet>
    </>
  );
}

// Rendered under DropdownMenuTrigger/TooltipTrigger `asChild`, so it must
// forward the ref and spread the props Radix attaches; swallowing them leaves
// the button inert.
const ModeSelectorTrigger = forwardRef<
  HTMLButtonElement,
  ComponentPropsWithoutRef<typeof Button> & {
    displayName: string;
    notApplied: boolean;
    triggerClassName?: string;
  }
>(function ModeSelectorTrigger({ displayName, notApplied, triggerClassName, ...props }, ref) {
  return (
    <Button
      {...props}
      ref={ref}
      variant="ghost"
      size="sm"
      data-testid="session-mode-selector"
      className={cn(
        "h-7 min-w-0 gap-1 overflow-hidden px-2 cursor-pointer whitespace-nowrap hover:bg-muted/40 [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:min-w-11",
        triggerClassName,
      )}
    >
      {notApplied && (
        <IconAlertTriangle
          className="h-3 w-3 text-amber-500 shrink-0"
          data-testid="session-mode-not-applied"
          aria-hidden
        />
      )}
      <span className="truncate text-xs">{displayName}</span>
      <IconChevronDown className="h-3 w-3 text-muted-foreground shrink-0" />
    </Button>
  );
});

function useModeSelectorDropdownState() {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const recentlyClosedRef = useRef(false);

  const handleDropdownOpenChange = useCallback((open: boolean) => {
    setDropdownOpen(open);
    if (!open) {
      recentlyClosedRef.current = true;
      setTooltipOpen(false);
      setTimeout(() => {
        recentlyClosedRef.current = false;
      }, 200);
    }
  }, []);

  const handleTooltipOpenChange = useCallback(
    (open: boolean) => {
      if (open && (dropdownOpen || recentlyClosedRef.current)) return;
      setTooltipOpen(open);
    },
    [dropdownOpen],
  );

  return {
    dropdownOpen,
    tooltipOpen,
    handleDropdownOpenChange,
    handleTooltipOpenChange,
  };
}

function useModeChangeHandler(
  sessionId: string | null,
  feedback: ReturnType<typeof usePickerConfigurationFeedback>,
  label: string,
) {
  return useCallback(
    async (modeId: string) => {
      if (!sessionId) return;
      const feedbackId = feedback.begin();
      const change = { configId: "mode", label, requested: modeId };
      try {
        const response = await setSessionMode(sessionId, modeId);
        feedback.showChanges(feedbackId, [{ ...change, result: response.configuration_result }]);
      } catch (error) {
        feedback.showChanges(feedbackId, [{ ...change, error }]);
      }
    },
    [feedback, label, sessionId],
  );
}

function resolveModeTooltipLabel({
  modeState,
  requestedName,
  displayName,
  defaultLabel,
  t,
}: {
  modeState: ModeSelectorState;
  requestedName?: string;
  displayName: string;
  defaultLabel: string;
  t: ReturnType<typeof useTranslation>["t"];
}) {
  if (!requestedName) return defaultLabel;
  return modeState.currentModeId
    ? t("task:sessionModeNotApplied", { requested: requestedName, effective: displayName })
    : t("task:sessionModeUnconfirmed", { requested: requestedName });
}

function resolveModeDisplayNames(
  modeState: ModeSelectorState,
  t: ReturnType<typeof useTranslation>["t"],
) {
  const currentMode = modeState.availableModes.find((mode) => mode.id === modeState.currentModeId);
  const displayName = currentMode?.name || modeState.currentModeId || t("common:unknown");
  // Show a request name only when the agent did not reach the requested mode.
  const requestedName = modeState.requestedModeId
    ? (modeState.availableModes.find((mode) => mode.id === modeState.requestedModeId)?.name ??
      formatModeName(modeState.requestedModeId))
    : undefined;
  return { displayName, requestedName };
}

function DesktopModeSelector({
  modeState,
  displayName,
  requestedName,
  triggerClassName,
  tooltipLabel,
  dropdownOpen,
  tooltipOpen,
  onDropdownOpenChange,
  onTooltipOpenChange,
  onModeChange,
}: {
  modeState: ModeSelectorState;
  displayName: string;
  requestedName?: string;
  triggerClassName?: string;
  tooltipLabel: string;
  dropdownOpen: boolean;
  tooltipOpen: boolean;
  onDropdownOpenChange: (open: boolean) => void;
  onTooltipOpenChange: (open: boolean) => void;
  onModeChange: (modeId: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenu open={dropdownOpen} onOpenChange={onDropdownOpenChange}>
      <Tooltip open={tooltipOpen} onOpenChange={onTooltipOpenChange}>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <ModeSelectorTrigger
              displayName={displayName}
              notApplied={Boolean(requestedName)}
              triggerClassName={triggerClassName}
            />
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent side="top">{tooltipLabel}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="start" side="top" className="min-w-[280px]">
        <DropdownMenuLabel>
          {t(modeState.thinking ? THINKING_LEVEL_LABEL : "task:availableModes")}
        </DropdownMenuLabel>
        {modeState.availableModes.map((mode) => (
          <DropdownMenuItem
            key={mode.id}
            onClick={() => onModeChange(mode.id)}
            className={cn(
              "relative min-h-11 cursor-pointer border border-transparent pr-7 sm:min-h-8",
              mode.id === modeState.currentModeId &&
                "border-primary/50 bg-card font-medium hover:bg-card",
            )}
          >
            <div className="min-w-0 flex-1">
              <div>{mode.name}</div>
              {mode.description && (
                <div className="text-xs text-muted-foreground">{mode.description}</div>
              )}
            </div>
            {mode.id === modeState.currentModeId && (
              <IconCheck className="absolute right-2 h-4 w-4" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export const ModeSelector = memo(function ModeSelector({
  sessionId,
  triggerClassName,
}: ModeSelectorProps) {
  const { t } = useTranslation();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const feedback = usePickerConfigurationFeedback(sessionId, "mode");
  const modeState = useModeSelectorState(sessionId);
  const { dropdownOpen, tooltipOpen, handleDropdownOpenChange, handleTooltipOpenChange } =
    useModeSelectorDropdownState();

  useEffect(() => {
    if (!sessionId || !modeState) {
      handleDropdownOpenChange(false);
    }
  }, [handleDropdownOpenChange, modeState, sessionId]);

  const handleModeChange = useModeChangeHandler(
    sessionId,
    feedback,
    translateModeLabel(t, modeState?.thinking),
  );

  if (!sessionId || !modeState) {
    return null;
  }

  const { displayName, requestedName } = resolveModeDisplayNames(modeState, t);

  if (isMobile || !isFinePointer) {
    return (
      <MobileModeSelector
        modeState={modeState}
        displayName={displayName}
        requestedName={requestedName}
        triggerClassName={triggerClassName}
        onModeChange={handleModeChange}
      />
    );
  }

  const tooltipLabel = resolveModeTooltipLabel({
    modeState,
    requestedName,
    displayName,
    defaultLabel: translateModeLabel(t, modeState.thinking),
    t,
  });

  return (
    <DesktopModeSelector
      modeState={modeState}
      displayName={displayName}
      requestedName={requestedName}
      triggerClassName={triggerClassName}
      tooltipLabel={tooltipLabel}
      dropdownOpen={dropdownOpen}
      tooltipOpen={tooltipOpen}
      onDropdownOpenChange={handleDropdownOpenChange}
      onTooltipOpenChange={handleTooltipOpenChange}
      onModeChange={handleModeChange}
    />
  );
});
