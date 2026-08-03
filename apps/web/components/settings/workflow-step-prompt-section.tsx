"use client";

import type { WorkflowStep } from "@/lib/types/http";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import {
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
  const { prompts } = useCustomPrompts();
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
      <div
        className="overflow-hidden rounded-md border"
        data-settings-dirty={!savedStep || localPrompt !== (savedStep.prompt ?? "")}
      >
        <ScriptEditor
          value={localPrompt}
          onChange={(value) => {
            if (readOnly) return;
            onLocalPromptChange(value);
            debouncedUpdatePrompt(value);
          }}
          language="markdown"
          height={computeEditorHeight(localPrompt)}
          lineNumbers="off"
          readOnly={readOnly}
          placeholders={stepPromptPlaceholders(t)}
          mentionPrompts={prompts}
        />
      </div>
      <p className="text-[11px] text-muted-foreground/60">
        {t("workflows:stepPromptUsageHint", {
          open: "{{",
          taskPrompt: "{{task_prompt}}",
        })}
      </p>
    </div>
  );
}
