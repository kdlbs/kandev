"use client";

import { useState } from "react";
import { IconInfoCircle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";

type JqlEditorProps = {
  composedJql: string;
  customJql: string | null;
  onApply: (jql: string) => void;
  onReset: () => void;
};

// Inline editor that exposes the JQL being sent to Jira. Users can tweak it
// for one-off advanced queries; pressing Reset returns to pill-driven mode.
export function JqlEditor({ composedJql, customJql, onApply, onReset }: JqlEditorProps) {
  const source = customJql ?? composedJql;
  const [draft, setDraft] = useState(source);
  const [prevSource, setPrevSource] = useState(source);
  // React-recommended pattern for resetting state when a prop changes: compare
  // in render and call setState — avoids an extra effect pass.
  if (source !== prevSource) {
    setPrevSource(source);
    setDraft(source);
  }

  const dirty = draft.trim() !== source.trim();
  const overriding = customJql !== null;

  return (
    <div className="px-6 py-3 border-b shrink-0 bg-muted/30 space-y-2">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <IconInfoCircle className="h-3.5 w-3.5" />
        {overriding ? (
          <span>Custom JQL override — pill filters are paused.</span>
        ) : (
          <span>JQL is generated from the pills above. Edit to override.</span>
        )}
      </div>
      <Textarea
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        spellCheck={false}
        className="font-mono text-xs min-h-[72px] bg-background"
      />
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          onClick={() => onApply(draft)}
          disabled={!dirty || !draft.trim()}
          className="cursor-pointer h-7 text-xs"
        >
          Apply
        </Button>
        {overriding && (
          <Button
            size="sm"
            variant="outline"
            onClick={onReset}
            className="cursor-pointer h-7 text-xs"
          >
            Reset to pills
          </Button>
        )}
      </div>
    </div>
  );
}
