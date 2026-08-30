"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Label } from "@kandev/ui/label";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import type { PlaceholderInfo } from "@/lib/types/automation";
import { toScriptPlaceholders } from "./automation-placeholders";

type PromptSectionProps = {
  value: string;
  isDirty: boolean;
  onChange: (value: string) => void;
  placeholders: PlaceholderInfo[];
};

export function PromptSection({ value, isDirty, onChange, placeholders }: PromptSectionProps) {
  const { t } = useTranslation();
  const scriptPlaceholders = useMemo(() => toScriptPlaceholders(placeholders), [placeholders]);

  return (
    <div className="space-y-3">
      <Label className="text-xs uppercase tracking-wider text-muted-foreground">
        {t("automations:instructionsLabel")}
      </Label>
      <SettingsPromptEditor
        value={value}
        onChange={onChange}
        language="plaintext"
        height="160px"
        placeholders={scriptPlaceholders}
        promptReferences
        isDirty={isDirty}
        dirtyLevel="container"
        testId="automation-prompt-editor"
      />
      {placeholders.length > 0 && (
        <div className="text-xs text-muted-foreground space-y-1">
          <p className="font-medium">{t("automations:availablePlaceholdersColon")}</p>
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
