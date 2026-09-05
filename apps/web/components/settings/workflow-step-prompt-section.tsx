"use client";

import type { WorkflowStep } from "@/lib/types/http";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { SettingsPromptEditor } from "./settings-prompt-editor";
import {
  hasOnEnterAction,
  HelpTip,
  stepPromptPlaceholders,
  PROMPT_TEMPLATES,
} from "./workflow-pipeline-editor-helpers";

export function StepPromptSection({
  step,
  savedStep,
  localPrompt,
  onLocalPromptChange,
  debouncedUpdatePrompt,
  readOnly,
}: {
  step: WorkflowStep;
  savedStep?: WorkflowStep;
  localPrompt: string;
  onLocalPromptChange: (prompt: string) => void;
  debouncedUpdatePrompt: (prompt: string) => void;
  readOnly: boolean;
}) {
  const { t } = useTranslation();
  const placeholders = useMemo(() => stepPromptPlaceholders(t), [t]);
  const showAutoStartWarning = localPrompt !== "" && !hasOnEnterAction(step, "auto_start_agent");
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5">
        <Label
          htmlFor={`${step.id}-prompt`}
          className="text-xs font-medium uppercase tracking-wider text-muted-foreground"
        >
          {t("workflows:stepPrompt")}
        </Label>
        <HelpTip text={t("workflows:stepPromptHelp", { taskPrompt: "{{task_prompt}}" })} />
      </div>
      {!readOnly && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-[11px] text-muted-foreground/60">{t("workflows:templates")}</span>
          {PROMPT_TEMPLATES.map((template) => (
            <button
              key={template.labelKey}
              type="button"
              onClick={() => {
                onLocalPromptChange(template.prompt);
                debouncedUpdatePrompt(template.prompt);
              }}
              className="cursor-pointer rounded-md border border-border bg-muted/50 px-2 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {t(template.labelKey)}
            </button>
          ))}
        </div>
      )}
      <SettingsPromptEditor
        value={localPrompt}
        onChange={(value) => {
          if (readOnly) return;
          onLocalPromptChange(value);
          debouncedUpdatePrompt(value);
        }}
        language="markdown"
        readOnly={readOnly}
        placeholders={placeholders}
        promptReferences
        testId={`workflow-step-prompt-${step.id}`}
        isDirty={!savedStep || localPrompt !== (savedStep.prompt ?? "")}
        dirtyLevel="container"
      />
      {showAutoStartWarning && (
        <div
          role="status"
          className="flex min-w-0 items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-800 dark:text-amber-100"
          data-testid="workflow-step-prompt-auto-start-warning"
        >
          <IconAlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <span className="min-w-0 text-pretty">{t("workflows:stepPromptAutoStartWarning")}</span>
        </div>
      )}
      <p className="text-[11px] text-muted-foreground/60">
        {t("workflows:stepPromptTaskDescriptionHint", {
          taskPrompt: "{{task_prompt}}",
        })}
      </p>
      <p className="text-[11px] text-muted-foreground/60">
        {t("workflows:stepPromptUsageHint", {
          open: "{{",
          taskPrompt: "{{task_prompt}}",
        })}
      </p>
    </div>
  );
}
