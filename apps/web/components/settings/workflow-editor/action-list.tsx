"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import type { WorkflowStep } from "@/lib/types/http";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import {
  getWorkflowActionCatalog,
  type WorkflowActionRecord,
  type WorkflowLifecycleTrigger,
} from "@/lib/workflows/workflow-action-catalog";

import { DesktopActionPicker, MobileActionPicker } from "./action-picker";
import { WorkflowActionRow } from "./action-row";

type WorkflowActionListProps = {
  trigger: WorkflowLifecycleTrigger;
  actions: WorkflowActionRecord[];
  step: WorkflowStep;
  steps: WorkflowStep[];
  selectedIndex?: number;
  children?: ReactNode;
  readOnly: boolean;
  onSelect: (index: number) => void;
  onAdd: (type: string) => void;
  mobile?: boolean;
};

const TRIGGER_LABEL_KEYS: Record<WorkflowLifecycleTrigger, string> = {
  on_enter: "workflows:onEnterActions",
  on_turn_start: "workflows:onTurnStartLabel",
  on_turn_complete: "workflows:onTurnCompleteLabel",
  on_exit: "workflows:onExit",
  on_children_completed: "workflows:childrenCompletedHelpAria",
};

const TRIGGER_DESCRIPTION_KEYS: Record<WorkflowLifecycleTrigger, string> = {
  on_enter: "workflows:runScriptHelp",
  on_turn_start: "workflows:onTurnStartHelp",
  on_turn_complete: "workflows:runScriptHelp",
  on_exit: "workflows:runScriptHelp",
  on_children_completed: "workflows:childrenCompletedHelp",
};

export function WorkflowActionList({
  trigger,
  actions,
  step,
  steps,
  selectedIndex,
  children,
  readOnly,
  onSelect,
  onAdd,
  mobile = false,
}: WorkflowActionListProps) {
  const { t } = useTranslation();
  const catalog = getWorkflowActionCatalog(trigger);
  const [open, setOpen] = useState(
    trigger === "on_enter" || trigger === "on_turn_complete" || actions.length > 0,
  );
  useEffect(() => {
    if (selectedIndex !== undefined) setOpen(true);
  }, [selectedIndex]);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <section
        className="min-w-0 space-y-2"
        data-testid={`workflow-action-list-${trigger}`}
        data-read-only={readOnly}
      >
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex min-h-11 w-full cursor-pointer items-center gap-2 text-left md:min-h-8 [@media(pointer:coarse)]:min-h-11"
            data-testid={`workflow-event-toggle-${trigger}`}
          >
            <IconChevronRight
              className={cn("h-3.5 w-3.5 shrink-0 text-muted-foreground", open && "rotate-90")}
              aria-hidden
            />
            <span className="min-w-0 flex-1 text-sm font-medium">
              {t(TRIGGER_LABEL_KEYS[trigger])}
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
              {t("workflows:stepActionCount", { count: actions.length })}
            </span>
          </button>
        </CollapsibleTrigger>
        <CollapsibleContent className="min-w-0 space-y-3 pl-5">
          <p className="text-xs text-muted-foreground">{t(TRIGGER_DESCRIPTION_KEYS[trigger])}</p>
          <div className="space-y-2">
            {actions.map((action, index) => (
              <div key={`${trigger}-${index}-${action.type}`} className="min-w-0">
                <WorkflowActionRow
                  action={action}
                  index={index}
                  step={step}
                  steps={steps}
                  readOnly={readOnly}
                  onSelect={() => onSelect(index)}
                />
                {selectedIndex === index && (
                  <div className="min-w-0 rounded-b-md border border-t-0 border-border p-3">
                    {children}
                  </div>
                )}
              </div>
            ))}
          </div>
          {!readOnly &&
            (mobile ? (
              <MobileActionPicker catalog={catalog} onAdd={onAdd} />
            ) : (
              <DesktopActionPicker catalog={catalog} onAdd={onAdd} />
            ))}
        </CollapsibleContent>
      </section>
    </Collapsible>
  );
}
