"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import { reviewWatchPlaceholders } from "@/components/github/review-watch-placeholders";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import { Trans, useTranslation } from "react-i18next";
import { useMemo } from "react";

// Example prompts shown verbatim in the help text. Interpolated as VALUES rather
// than written into the catalog: they contain a literal `{{task_prompt}}` token,
// which i18next would read as an interpolation and substitute away, and the
// pseudo-locale would transliterate the URL.
const STEP_PROMPT_EXAMPLE = "Analyze the task: {{task_prompt}}";
const FINAL_PROMPT_EXAMPLE = "Analyze the task: Pull Request ready for review: https://...";

// Pulled out of review-watch-dialog.tsx to keep that file under the 600-line
// linter cap; the prompt editor + its placeholder help bubble are co-owned by
// this single screen and aren't reused elsewhere.

function PlaceholdersHelp() {
  const { t } = useTranslation();
  const placeholders = useMemo(() => reviewWatchPlaceholders(t), [t]);
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">{t("github:availablePlaceholders")}</p>
          <ul className="text-xs space-y-0.5">
            {placeholders.map((p) => (
              <li key={p.key}>
                <code className="text-[10px] bg-white/15 px-1 rounded">{`{{${p.key}}}`}</code>{" "}
                <span className="opacity-70">{p.description}</span>
              </li>
            ))}
          </ul>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export function ReviewWatchPromptField({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  const { t } = useTranslation();
  const { prompts } = useCustomPrompts();
  const placeholders = useMemo(() => reviewWatchPlaceholders(t), [t]);
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>{t("github:taskPrompt")}</Label>
        <PlaceholdersHelp />
      </div>
      <p className="text-xs text-muted-foreground">
        {/* The `{{` and `@` sigils are passed as values so they never reach the
            catalog, where i18next would read `{{` as an interpolation opener. */}
        {t("github:reviewWatchPromptHelp", { token: "{{", mention: "@" })}
      </p>
      <div className="rounded-md border border-border overflow-hidden">
        <ScriptEditor
          value={value}
          onChange={onChange}
          language="markdown"
          height={computeEditorHeight(value)}
          lineNumbers="off"
          placeholders={placeholders}
          mentionPrompts={prompts}
        />
      </div>
      <p className="text-xs text-muted-foreground/70">
        <Trans
          i18nKey="github:workflowStepPromptWrapHelp"
          values={{ stepPrompt: STEP_PROMPT_EXAMPLE, finalPrompt: FINAL_PROMPT_EXAMPLE }}
        >
          The workflow step prompt wraps this prompt. For example, if the step prompt is{" "}
          <code className="text-[10px] bg-muted px-1 rounded">{"{{stepPrompt}}"}</code>, the final
          prompt becomes{" "}
          <code className="text-[10px] bg-muted px-1 rounded">{"{{finalPrompt}}"}</code>
        </Trans>
      </p>
    </div>
  );
}
