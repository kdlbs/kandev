"use client";

import type { ReactNode } from "react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import {
  computeEditorHeight,
  ScriptEditor,
} from "@/components/settings/profile-edit/script-editor";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";

const EMPTY_EXCLUDED_PROMPT_IDS: string[] = [];

export type SettingsPromptEditorProps = {
  value: string;
  onChange: (value: string) => void;
  placeholders?: ScriptPlaceholder[];
  promptReferences?: boolean;
  excludedPromptIds?: string[];
  language?: "markdown" | "plaintext";
  height?: string | number;
  minLines?: number;
  readOnly?: boolean;
  ariaLabel?: string;
  testId?: string;
  isDirty?: boolean;
  dirtyLevel?: "field" | "container";
  help?: ReactNode;
};

function completionHint(
  t: (key: string, options?: Record<string, string>) => string,
  hasPlaceholders: boolean,
  hasPromptReferences: boolean,
) {
  if (hasPlaceholders && hasPromptReferences) {
    return t("settings:promptEditorBothHint", { placeholder: "{{", mention: "@" });
  }
  if (hasPlaceholders) {
    return t("settings:promptEditorPlaceholderHint", { placeholder: "{{" });
  }
  if (hasPromptReferences) {
    return t("settings:promptEditorPromptReferenceHint", { mention: "@" });
  }
  return null;
}

export function SettingsPromptEditor({
  value,
  onChange,
  placeholders = [],
  promptReferences = false,
  excludedPromptIds = EMPTY_EXCLUDED_PROMPT_IDS,
  language = "markdown",
  height,
  minLines = 3,
  readOnly = false,
  ariaLabel,
  testId,
  isDirty,
  dirtyLevel = "container",
  help,
}: SettingsPromptEditorProps) {
  const { t } = useTranslation();
  const { prompts, loaded, loading } = useCustomPrompts({ enabled: promptReferences });
  const excludedIds = useMemo(() => new Set(excludedPromptIds), [excludedPromptIds]);
  const mentionPrompts = useMemo(() => {
    if (!loaded || loading) return [];
    return prompts.filter((prompt) => !excludedIds.has(prompt.id));
  }, [excludedIds, loaded, loading, prompts]);
  const hint = completionHint(t, placeholders.length > 0, promptReferences);

  return (
    <div
      className="settings-prompt-editor space-y-1.5"
      data-settings-dirty={isDirty}
      data-settings-dirty-level={dirtyLevel}
    >
      <div className="rounded-md border border-border overflow-hidden" data-testid={testId}>
        <ScriptEditor
          value={value}
          onChange={onChange}
          language={language}
          height={height ?? computeEditorHeight(value, minLines)}
          lineNumbers="off"
          placeholders={placeholders}
          mentionPrompts={promptReferences ? mentionPrompts : undefined}
          readOnly={readOnly}
          ariaLabel={ariaLabel ?? t("settings:promptEditorAriaLabel")}
          testId={testId ? `${testId}-editor` : undefined}
        />
      </div>
      {hint && <p className="text-[11px] text-muted-foreground/60">{hint}</p>}
      {help}
    </div>
  );
}
