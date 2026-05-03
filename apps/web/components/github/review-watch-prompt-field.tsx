"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import { REVIEW_WATCH_PLACEHOLDERS } from "@/components/github/review-watch-placeholders";

// Pulled out of review-watch-dialog.tsx to keep that file under the 600-line
// linter cap; the prompt editor + its placeholder help bubble are co-owned by
// this single screen and aren't reused elsewhere.

function PlaceholdersHelp() {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-xs" align="start">
          <p className="text-xs font-medium mb-1">Available placeholders:</p>
          <ul className="text-xs space-y-0.5">
            {REVIEW_WATCH_PLACEHOLDERS.map((p) => (
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
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1.5">
        <Label>Task Prompt</Label>
        <PlaceholdersHelp />
      </div>
      <p className="text-xs text-muted-foreground">
        The prompt sent to the agent for each new PR. Type {"{{"} to insert placeholders.
      </p>
      <div className="rounded-md border border-border overflow-hidden">
        <ScriptEditor
          value={value}
          onChange={onChange}
          language="markdown"
          height={computeEditorHeight(value)}
          lineNumbers="off"
          placeholders={REVIEW_WATCH_PLACEHOLDERS}
        />
      </div>
      <p className="text-xs text-muted-foreground/70">
        The workflow step prompt wraps this prompt. For example, if the step prompt is{" "}
        <code className="text-[10px] bg-muted px-1 rounded">
          {"Analyze the task: {{task_prompt}}"}
        </code>
        , the final prompt becomes{" "}
        <code className="text-[10px] bg-muted px-1 rounded">
          {"Analyze the task: Pull Request ready for review: https://..."}
        </code>
      </p>
    </div>
  );
}
