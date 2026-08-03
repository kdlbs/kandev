import type React from "react";
import type { Icon } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import {
  IconArrowRight,
  IconClipboard,
  IconDoorExit,
  IconMessageForward,
  IconRefresh,
  IconRobot,
  IconUserCog,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";

type StepCapabilityIconsProps = {
  events?: KanbanStepEvents;
  agentProfileId?: string;
  className?: string;
  fallback?: React.ReactNode;
};

const TRANSITION_TYPES = ["move_to_next", "move_to_previous", "move_to_step"];

type CapabilityDef = {
  key: string;
  icon: Icon;
  /** Catalog key: `key` and the action `type`s it checks are wire values. */
  tooltipKey: string;
  check: (events: KanbanStepEvents) => boolean;
};

const CAPABILITIES: CapabilityDef[] = [
  {
    key: "onTurnStart",
    icon: IconMessageForward,
    tooltipKey: "workflows:onUserMessage",
    check: (e) => e.on_turn_start?.some((a) => TRANSITION_TYPES.includes(a.type)) ?? false,
  },
  {
    key: "autoStart",
    icon: IconRobot,
    tooltipKey: "workflows:autoStartAgent",
    check: (e) => e.on_enter?.some((a) => a.type === "auto_start_agent") ?? false,
  },
  {
    key: "planMode",
    icon: IconClipboard,
    tooltipKey: "workflows:planMode",
    check: (e) => e.on_enter?.some((a) => a.type === "enable_plan_mode") ?? false,
  },
  {
    key: "resetContext",
    icon: IconRefresh,
    tooltipKey: "workflows:resetAgentContext",
    check: (e) => e.on_enter?.some((a) => a.type === "reset_agent_context") ?? false,
  },
  {
    key: "transition",
    icon: IconArrowRight,
    tooltipKey: "workflows:autoTransition",
    check: (e) => e.on_turn_complete?.some((a) => TRANSITION_TYPES.includes(a.type)) ?? false,
  },
  {
    key: "onExit",
    icon: IconDoorExit,
    tooltipKey: "workflows:onExitActions",
    check: (e) => (e.on_exit?.length ?? 0) > 0,
  },
];

export function StepCapabilityIcons({
  events,
  agentProfileId,
  className,
  fallback,
}: StepCapabilityIconsProps) {
  const { t } = useTranslation();
  const defaultClassName = "flex items-center gap-1.5 text-muted-foreground";
  const activeCapabilities = events ? CAPABILITIES.filter((cap) => cap.check(events)) : [];
  const hasAgentProfile = Boolean(agentProfileId);

  if (activeCapabilities.length === 0 && !hasAgentProfile) {
    return fallback ? <div className={className ?? defaultClassName}>{fallback}</div> : null;
  }

  return (
    <div className={className ?? defaultClassName}>
      {hasAgentProfile && (
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <IconUserCog className="h-3.5 w-3.5" />
            </TooltipTrigger>
            <TooltipContent>{t("workflows:customAgentProfile")}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )}
      {activeCapabilities.map((cap) => {
        const IconComponent = cap.icon;
        return (
          <TooltipProvider key={cap.key}>
            <Tooltip>
              <TooltipTrigger asChild>
                <IconComponent className="h-3.5 w-3.5" />
              </TooltipTrigger>
              <TooltipContent>{t(cap.tooltipKey)}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        );
      })}
    </div>
  );
}
