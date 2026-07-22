import type { GitLabActionPreset } from "@/lib/types/gitlab";
import {
  iconForPresetKey,
  interpolatePromptTemplate,
} from "@/components/github/my-github/action-presets";
import type { GitLabTaskPreset } from "./quick-task-launcher";

export function toGitLabTaskPreset(preset: GitLabActionPreset): GitLabTaskPreset {
  return {
    id: preset.id,
    label: preset.label,
    hint: preset.hint,
    icon: iconForPresetKey(preset.icon),
    prompt: ({ url, title }) => interpolatePromptTemplate(preset.prompt_template, { url, title }),
  };
}
