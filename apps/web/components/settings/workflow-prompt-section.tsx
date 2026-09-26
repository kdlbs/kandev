"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronRight } from "@tabler/icons-react";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Label } from "@kandev/ui/label";
import { cn } from "@/lib/utils";
import type { Workflow } from "@/lib/types/http";
import { SettingsPromptEditor } from "./settings-prompt-editor";
import { HelpTip } from "./workflow-pipeline-editor-helpers";
import { isWorkflowFieldDirty } from "./workflow-dirty-state";

type WorkflowPromptSectionProps = {
  workflow: Workflow;
  savedWorkflow?: Workflow;
  readOnly?: boolean;
  onUpdate: (prompt: string) => void;
};

/**
 * Optional workflow-level agent prompt editor.
 * Collapsed when empty so the common "no shared instructions" case stays compact;
 * opens automatically when a non-empty prompt is loaded or the user expands it.
 */
export function WorkflowPromptSection({
  workflow,
  savedWorkflow,
  readOnly,
  onUpdate,
}: WorkflowPromptSectionProps) {
  const { t } = useTranslation();
  const prompt = workflow.prompt ?? "";
  const hasPrompt = prompt.trim().length > 0;
  const [open, setOpen] = useState(hasPrompt);

  // Keep expanded when content arrives (e.g. load/import/dirty restore).
  useEffect(() => {
    if (hasPrompt) setOpen(true);
  }, [hasPrompt]);

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="space-y-1.5">
      <div className="flex items-center gap-1">
        <CollapsibleTrigger
          type="button"
          className="flex cursor-pointer items-center gap-1 rounded-md px-1 py-0.5 text-left hover:bg-muted/50"
          data-testid="workflow-prompt-toggle"
        >
          <IconChevronRight
            className={cn(
              "h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
              open && "rotate-90",
            )}
          />
          <Label className="cursor-pointer">{t("workflows:workflowPrompt")}</Label>
        </CollapsibleTrigger>
        <HelpTip text={t("workflows:workflowPromptHelp")} />
        {!open && !hasPrompt && (
          <span className="text-xs text-muted-foreground">
            {t("workflows:workflowPromptCollapsedHint")}
          </span>
        )}
        {!open && hasPrompt && (
          <span className="truncate text-xs text-muted-foreground" title={prompt}>
            {t("workflows:workflowPromptSetHint")}
          </span>
        )}
      </div>
      <CollapsibleContent className="space-y-1.5">
        <SettingsPromptEditor
          value={prompt}
          onChange={onUpdate}
          promptReferences
          readOnly={readOnly}
          ariaLabel={t("workflows:workflowPrompt")}
          testId="workflow-prompt-input"
          isDirty={isWorkflowFieldDirty(workflow, savedWorkflow, "prompt")}
          dirtyLevel="field"
          help={
            <p className="text-xs text-muted-foreground">
              {t("workflows:workflowPromptPlaceholder")}
            </p>
          }
        />
      </CollapsibleContent>
    </Collapsible>
  );
}
