import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";
import type { PlaceholderInfo } from "@/lib/types/automation";

/**
 * Converts backend PlaceholderInfo[] to ScriptPlaceholder[] for the Monaco editor.
 * The executor_types field is empty since automation placeholders apply to all executors.
 */
export function toScriptPlaceholders(placeholders: PlaceholderInfo[]): ScriptPlaceholder[] {
  return placeholders.map((p) => ({
    key: p.key,
    description: p.description,
    example: p.example,
    executor_types: [],
  }));
}
