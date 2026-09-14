"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import {
  getWorkflowActionCatalog,
  type WorkflowActionRecord,
  type WorkflowActionDescriptor,
  type WorkflowLifecycleTrigger,
} from "@/lib/workflows/workflow-action-catalog";

type WorkflowActionListProps = {
  trigger: WorkflowLifecycleTrigger;
  actions: WorkflowActionRecord[];
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

const ADD_ACTION_LABEL_KEY = "workflows:addAction";

export function WorkflowActionList({
  trigger,
  actions,
  readOnly,
  onSelect,
  onAdd,
  mobile = false,
}: WorkflowActionListProps) {
  const { t } = useTranslation();
  const catalog = getWorkflowActionCatalog(trigger);
  return (
    <section
      className="space-y-3 rounded-lg border border-border/70 p-3"
      data-testid={`workflow-action-list-${trigger}`}
      data-read-only={readOnly}
    >
      <div>
        <h4 className="text-sm font-medium">{t(TRIGGER_LABEL_KEYS[trigger])}</h4>
        <p className="text-xs text-muted-foreground">{t(TRIGGER_DESCRIPTION_KEYS[trigger])}</p>
      </div>
      <div className="space-y-2">
        {actions.map((action, index) => (
          <WorkflowActionRow
            key={`${trigger}-${index}-${action.type}`}
            action={action}
            index={index}
            readOnly={readOnly}
            onSelect={() => onSelect(index)}
          />
        ))}
        {actions.length === 0 && (
          <p className="rounded-md border border-dashed border-border/70 px-3 py-3 text-xs text-muted-foreground">
            {t("workflows:noAutomationActions")}
          </p>
        )}
      </div>
      {!readOnly &&
        (mobile ? (
          <MobileActionPicker catalog={catalog} onAdd={onAdd} />
        ) : (
          <DesktopActionPicker catalog={catalog} onAdd={onAdd} />
        ))}
    </section>
  );
}

function DesktopActionPicker({
  catalog,
  onAdd,
}: {
  catalog: readonly WorkflowActionDescriptor[];
  onAdd: (type: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <IconPlus className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
      <select
        aria-label={t(ADD_ACTION_LABEL_KEY)}
        className="min-h-11 min-w-0 flex-1 rounded-md border border-input bg-background px-3 text-sm"
        value=""
        onChange={(event) => {
          if (event.target.value) onAdd(event.target.value);
        }}
      >
        <option value="" disabled>
          {t(ADD_ACTION_LABEL_KEY)}
        </option>
        {catalog.map((item) => (
          <option key={item.type} value={item.type}>
            {t(item.labelKey)}
          </option>
        ))}
      </select>
    </div>
  );
}

function MobileActionPicker({
  catalog,
  onAdd,
}: {
  catalog: readonly WorkflowActionDescriptor[];
  onAdd: (type: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <Drawer open={open} onOpenChange={setOpen}>
      <Button
        type="button"
        variant="outline"
        className="min-h-11 w-full cursor-pointer"
        onClick={() => setOpen(true)}
      >
        <IconPlus className="mr-1.5 h-4 w-4" />
        {t(ADD_ACTION_LABEL_KEY)}
      </Button>
      <DrawerContent data-testid="workflow-mobile-action-picker">
        <DrawerHeader>
          <DrawerTitle>{t(ADD_ACTION_LABEL_KEY)}</DrawerTitle>
          <DrawerDescription>{t("workflows:chooseActionDescription")}</DrawerDescription>
        </DrawerHeader>
        <div className="grid gap-2 overflow-y-auto px-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
          {catalog.map((item) => (
            <Button
              key={item.type}
              type="button"
              variant="outline"
              className="min-h-11 cursor-pointer justify-start"
              onClick={() => {
                onAdd(item.type);
                setOpen(false);
              }}
            >
              {t(item.labelKey)}
            </Button>
          ))}
        </div>
      </DrawerContent>
    </Drawer>
  );
}

function WorkflowActionRow({
  action,
  index,
  readOnly,
  onSelect,
}: {
  action: WorkflowActionRecord;
  index: number;
  readOnly: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const descriptor = [
    ...getWorkflowActionCatalog("on_enter"),
    ...getWorkflowActionCatalog("on_turn_start"),
    ...getWorkflowActionCatalog("on_turn_complete"),
    ...getWorkflowActionCatalog("on_exit"),
    ...getWorkflowActionCatalog("on_children_completed"),
  ].find((item) => item.type === action.type);
  const summary = actionSummary(action, descriptor, t);
  return (
    <button
      type="button"
      className="flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-md border border-border px-3 text-left text-sm transition-colors hover:border-primary/50"
      aria-label={t("workflows:selectActionNumber", { index: index + 1 })}
      onClick={onSelect}
      data-read-only={readOnly}
    >
      <span className="w-5 shrink-0 text-xs tabular-nums text-muted-foreground">{index + 1}</span>
      <span className="min-w-0 flex-1 truncate font-mono">{summary}</span>
      <IconChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
    </button>
  );
}

function actionSummary(
  action: WorkflowActionRecord,
  descriptor: WorkflowActionDescriptor | undefined,
  translate: (key: string) => string,
): string {
  if (action.type === "run_script" && typeof action.config?.command === "string") {
    return action.config.command;
  }
  if (descriptor) return translate(descriptor.labelKey);
  return action.type;
}
