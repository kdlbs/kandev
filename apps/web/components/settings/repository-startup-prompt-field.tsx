"use client";

import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { Repository } from "@/lib/types/http";

type StartupPromptFieldProps = {
  repositoryId: string;
  startupPrompt: string;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

export function RepositoryStartupPromptField({
  repositoryId,
  startupPrompt,
  onUpdate,
}: StartupPromptFieldProps) {
  return (
    <div className="space-y-2">
      <Label>Startup Prompt</Label>
      <Textarea
        value={startupPrompt}
        onChange={(e) => onUpdate(repositoryId, { startup_prompt: e.target.value })}
        placeholder="Read {{TICKET_URL}} and start work on {{TASK_TITLE}}."
        rows={3}
        className="font-mono text-sm"
      />
      <p className="text-xs text-muted-foreground">
        Pre-fills new task descriptions for this repository. Supports{" "}
        <code className="px-1 py-0.5 bg-muted rounded">{"{{TICKET_ID}}"}</code>,{" "}
        <code className="px-1 py-0.5 bg-muted rounded">{"{{TICKET_URL}}"}</code>,{" "}
        <code className="px-1 py-0.5 bg-muted rounded">{"{{TICKET_PROVIDER}}"}</code>, and{" "}
        <code className="px-1 py-0.5 bg-muted rounded">{"{{TASK_TITLE}}"}</code>. Lines whose
        placeholders don&apos;t resolve are dropped from the pre-fill.
      </p>
    </div>
  );
}
