"use client";

import { useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { ScriptEditor } from "@/components/settings/profile-edit/script-editor";
import type { PlaceholderInfo } from "@/lib/types/automation";
import { toScriptPlaceholders } from "./automation-placeholders";

type PromptSectionProps = {
  value: string;
  onChange: (value: string) => void;
  placeholders: PlaceholderInfo[];
};

export function PromptSection({ value, onChange, placeholders }: PromptSectionProps) {
  const scriptPlaceholders = useMemo(() => toScriptPlaceholders(placeholders), [placeholders]);

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">Instructions</Label>
      <ScriptEditor
        value={value}
        onChange={onChange}
        language="plaintext"
        height="160px"
        placeholders={scriptPlaceholders}
      />
      {placeholders.length > 0 && (
        <div className="text-xs text-muted-foreground space-y-1">
          <p className="font-medium">Available placeholders:</p>
          <div className="flex flex-wrap gap-x-3 gap-y-1">
            {placeholders.map((p) => (
              <span key={p.key} title={p.description}>
                <code className="text-[11px]">{`{{${p.key}}}`}</code>
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
