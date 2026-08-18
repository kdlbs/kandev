"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { HelpTip } from "./workflow-pipeline-editor-helpers";
import type { WorkflowStep } from "@/lib/types/http";

/**
 * Per-step coordinator monitoring configuration.
 * Selected is true when this step should be monitored by the coordinator.
 * Prompt is an optional multiline instruction sent to the coordinator
 * every time this step is checked.
 */
export interface StepMonitorConfig {
  selected: boolean;
  prompt: string;
}

export type MonitorConfigMap = Record<string, StepMonitorConfig>;

export interface CoordinatorMonitorSectionProps {
  /** Unique id for the containing workflow, used for stable react keys. */
  workflowId: string;
  /** The workflow steps to render monitoring toggles for. */
  steps: WorkflowStep[];
  /** Current monitor configuration, keyed by step id. */
  config: MonitorConfigMap;
  /** Called when any checkbox or prompt changes. */
  onChange: (config: MonitorConfigMap) => void;
  /** Disables all inputs. */
  disabled?: boolean;
}

/**
 * Hook that manages local draft state for the coordinator monitor config.
 *
 * Returns the current draft config and a handler that updates one step at
 * a time, so the parent can integrate it with a save coordinator.
 */
export function useCoordinatorMonitorDraft(
  initialConfig: MonitorConfigMap,
  onChange: (config: MonitorConfigMap) => void,
) {
  const [draft, setDraft] = useState<MonitorConfigMap>(initialConfig);
  const handleStepChange = useCallback(
    (stepId: string, patch: Partial<StepMonitorConfig>) => {
      setDraft((prev) => {
        const next: MonitorConfigMap = { ...prev };
        const existing = next[stepId] ?? { selected: false, prompt: "" };
        next[stepId] = { ...existing, ...patch };
        onChange(next);
        return next;
      });
    },
    [onChange],
  );
  return { draft, setDraft, handleStepChange };
}

/**
 * Coordinator monitoring section for a single workflow.
 *
 * Renders a card listing every workflow step with a checkbox and
 * optional per-step prompt textarea.  Acts as a pure controlled
 * component: the parent owns the config state and passes it down.
 *
 * In production usage the config is persisted via the plugin's
 * user-state storage keyed by (workspace_id, workflow_id, step_id),
 * but this component only reads/writes the provided MonitorConfigMap.
 */
export function CoordinatorMonitorSection({
  workflowId,
  steps,
  config,
  onChange,
  disabled = false,
}: CoordinatorMonitorSectionProps) {
  const { t } = useTranslation();

  const handleSelect = useCallback(
    (stepId: string, selected: boolean) => {
      onChange({
        ...config,
        [stepId]: { ...(config[stepId] ?? { selected: false, prompt: "" }), selected },
      });
    },
    [config, onChange],
  );

  const handlePrompt = useCallback(
    (stepId: string, prompt: string) => {
      onChange({
        ...config,
        [stepId]: { ...(config[stepId] ?? { selected: false, prompt: "" }), prompt },
      });
    },
    [config, onChange],
  );

  return (
    <Card className="mt-4">
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium flex items-center gap-2">
          {t("workflows:coordinatorMonitoring")}
          <HelpTip text={t("workflows:coordinatorMonitoringHelp")} />
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {steps.length === 0 && (
          <p className="text-sm text-muted-foreground">{t("workflows:noStepsToMonitor")}</p>
        )}
        {steps.map((step) => {
          const stepConfig = config[step.id] ?? { selected: false, prompt: "" };
          return (
            <div key={`${workflowId}-${step.id}`} className="space-y-2">
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`monitor-${workflowId}-${step.id}`}
                  checked={stepConfig.selected}
                  disabled={disabled}
                  onCheckedChange={(checked) => handleSelect(step.id, checked === true)}
                />
                <Label
                  htmlFor={`monitor-${workflowId}-${step.id}`}
                  className="text-sm font-normal cursor-pointer"
                >
                  {step.name}
                </Label>
              </div>
              {stepConfig.selected && (
                <div className="ml-6">
                  <Textarea
                    value={stepConfig.prompt}
                    disabled={disabled}
                    placeholder={t("workflows:coordinatorStepPromptPlaceholder")}
                    className="min-h-[4rem] max-w-lg"
                    onChange={(e) => handlePrompt(step.id, e.target.value)}
                  />
                </div>
              )}
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
