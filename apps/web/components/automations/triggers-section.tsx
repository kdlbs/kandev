"use client";

import { useMemo } from "react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import type { AutomationTrigger, TriggerType, TriggerTypeInfo } from "@/lib/types/automation";
import { ScheduleSelector } from "./schedule-selector";
import { TriggerCard } from "./trigger-card";
import { TriggerPicker } from "./trigger-picker";

type TriggersSectionProps = {
  triggers: AutomationTrigger[];
  automationId: string | null;
  triggerTypes: TriggerTypeInfo[];
  onAddTrigger: (type: TriggerType, config: Record<string, unknown>) => void;
  onUpdateTrigger: (triggerId: string, config: Record<string, unknown>) => void;
  onToggleTrigger: (triggerId: string, enabled: boolean) => void;
  onDeleteTrigger: (triggerId: string) => void;
  // oneTimeWebhookSecret is the plaintext webhook secret from the create
  // response. Forwarded to the webhook trigger card so the user can see
  // and copy it once. Null after reload — they must reveal it then.
  oneTimeWebhookSecret?: string | null;
};

export function TriggersSection({
  triggers,
  automationId,
  triggerTypes,
  onAddTrigger,
  onUpdateTrigger,
  onToggleTrigger,
  onDeleteTrigger,
  oneTimeWebhookSecret,
}: TriggersSectionProps) {
  const webhookTrigger = useMemo(() => triggers.find((t) => t.type === "webhook"), [triggers]);
  const isWebhookMode = !!webhookTrigger;

  const scheduleTrigger = useMemo(() => triggers.find((t) => t.type === "scheduled"), [triggers]);
  const conditionTrigger = useMemo(
    () => triggers.find((t) => t.type !== "scheduled" && t.type !== "webhook"),
    [triggers],
  );

  const handleScheduleChange = (config: Record<string, unknown>) => {
    if (scheduleTrigger) {
      onUpdateTrigger(scheduleTrigger.id, config);
    } else {
      onAddTrigger("scheduled", config);
    }
  };

  const handleAddWebhook = () => {
    // Switching to webhook mode is exclusive: any pre-existing scheduled /
    // condition triggers must be deleted, otherwise they keep firing in the
    // background while the UI only shows the webhook trigger.
    if (scheduleTrigger) onDeleteTrigger(scheduleTrigger.id);
    if (conditionTrigger) onDeleteTrigger(conditionTrigger.id);
    onAddTrigger("webhook", {});
  };

  const handleRemoveWebhook = () => {
    if (webhookTrigger) onDeleteTrigger(webhookTrigger.id);
  };

  if (isWebhookMode) {
    return (
      <div className="space-y-3">
        <TriggerCard
          trigger={webhookTrigger}
          automationId={automationId}
          onUpdate={(config) => onUpdateTrigger(webhookTrigger.id, config)}
          onToggleEnabled={(enabled) => onToggleTrigger(webhookTrigger.id, enabled)}
          onDelete={handleRemoveWebhook}
          oneTimeWebhookSecret={oneTimeWebhookSecret}
        />
        <SwitchModeButton label="Switch to scheduled" onClick={handleRemoveWebhook} />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <ScheduleArea scheduleTrigger={scheduleTrigger} onScheduleChange={handleScheduleChange} />
      <ConditionArea
        trigger={conditionTrigger}
        automationId={automationId}
        triggerTypes={triggerTypes}
        onAddTrigger={onAddTrigger}
        onUpdateTrigger={onUpdateTrigger}
        onToggleTrigger={onToggleTrigger}
        onDeleteTrigger={onDeleteTrigger}
      />
      <SwitchModeButton label="Or use a webhook instead" onClick={handleAddWebhook} />
    </div>
  );
}

function ScheduleArea({
  scheduleTrigger,
  onScheduleChange,
}: {
  scheduleTrigger: AutomationTrigger | undefined;
  onScheduleChange: (config: Record<string, unknown>) => void;
}) {
  return (
    <div className="space-y-2">
      <Label className="text-xs font-medium">Schedule</Label>
      <ScheduleSelector config={scheduleTrigger?.config ?? null} onChange={onScheduleChange} />
    </div>
  );
}

function ConditionArea({
  trigger,
  automationId,
  triggerTypes,
  onAddTrigger,
  onUpdateTrigger,
  onToggleTrigger,
  onDeleteTrigger,
}: {
  trigger: AutomationTrigger | undefined;
  automationId: string | null;
  triggerTypes: TriggerTypeInfo[];
  onAddTrigger: (type: TriggerType, config: Record<string, unknown>) => void;
  onUpdateTrigger: (triggerId: string, config: Record<string, unknown>) => void;
  onToggleTrigger: (triggerId: string, enabled: boolean) => void;
  onDeleteTrigger: (triggerId: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label className="text-xs font-medium">Watch for</Label>
      {trigger ? (
        <div className="space-y-2">
          <TriggerCard
            trigger={trigger}
            automationId={automationId}
            onUpdate={(config) => onUpdateTrigger(trigger.id, config)}
            onToggleEnabled={(enabled) => onToggleTrigger(trigger.id, enabled)}
            onDelete={() => onDeleteTrigger(trigger.id)}
          />
        </div>
      ) : (
        <TriggerPicker triggerTypes={triggerTypes} onSelect={onAddTrigger} />
      )}
    </div>
  );
}

function SwitchModeButton({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <Button
      variant="ghost"
      size="sm"
      className="cursor-pointer text-xs text-muted-foreground"
      onClick={onClick}
    >
      {label}
    </Button>
  );
}
