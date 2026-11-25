"use client";

import { useMemo, useState } from "react";
import { IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { Label } from "@kandev/ui/label";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import type { PlaceholderInfo } from "@/lib/types/automation";
import { toScriptPlaceholders } from "./automation-placeholders";

type PromptSectionProps = {
  value: string;
  isDirty: boolean;
  onChange: (value: string) => void;
  placeholders: PlaceholderInfo[];
  /** Overrides the default instructions heading. */
  label?: string;
  description?: string;
  testId?: string;
};

export function PromptSection({
  value,
  isDirty,
  onChange,
  placeholders,
  label,
  description,
  testId = "automation-prompt-editor",
}: PromptSectionProps) {
  const { t } = useTranslation();
  const scriptPlaceholders = useMemo(() => toScriptPlaceholders(placeholders), [placeholders]);

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        {label ?? t("automations:instructionsLabel")}
      </Label>
      {description ? <p className="text-xs text-muted-foreground">{description}</p> : null}
      <SettingsPromptEditor
        value={value}
        onChange={onChange}
        language="plaintext"
        height="160px"
        placeholders={scriptPlaceholders}
        promptReferences
        isDirty={isDirty}
        dirtyLevel="container"
        testId={testId}
      />
    </div>
  );
}

/**
 * The trigger's placeholders, listed once for every prompt on the page. The
 * editors already offer them when the user types {{, so the list starts closed.
 */
export function PlaceholderList({ placeholders }: { placeholders: PlaceholderInfo[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  if (placeholders.length === 0) return null;
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="space-y-2">
      <CollapsibleTrigger className="flex min-h-7 cursor-pointer items-center gap-1 text-xs text-muted-foreground hover:text-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
        <IconChevronRight
          aria-hidden
          className={`h-3.5 w-3.5 transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`}
        />
        {t("automations:showPlaceholders")}
      </CollapsibleTrigger>
      <CollapsibleContent className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {placeholders.map((p) => (
          <span key={p.key} title={p.description}>
            <code className="text-[11px]">{`{{${p.key}}}`}</code>
          </span>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}
