import type { Icon } from "@tabler/icons-react";
import { iconForIntegrationPreset } from "@/components/integrations/integration-preset-icons";
import type {
  GitHubActionPreset,
  GitHubActionPresetIcon,
  GitHubActionPresets,
} from "@/lib/types/github";
import type { TaskPreset } from "./quick-task-launcher";

// `key` is the persisted GitHubActionPresetIcon enum; only the label is copy, so
// it travels as a catalog key resolved at render (module-scope `t()` would
// freeze at the boot locale — see docs/i18n.md).
export const PRESET_ICON_CHOICES: {
  key: GitHubActionPresetIcon;
  icon: Icon;
  labelKey: string;
}[] = [
  { key: "eye", icon: iconForIntegrationPreset("eye"), labelKey: "github:presetIconEye" },
  {
    key: "message",
    icon: iconForIntegrationPreset("message"),
    labelKey: "github:presetIconMessage",
  },
  { key: "tool", icon: iconForIntegrationPreset("tool"), labelKey: "github:presetIconTool" },
  { key: "code", icon: iconForIntegrationPreset("code"), labelKey: "github:presetIconCode" },
  { key: "search", icon: iconForIntegrationPreset("search"), labelKey: "github:presetIconSearch" },
  { key: "bug", icon: iconForIntegrationPreset("bug"), labelKey: "github:presetIconBug" },
  {
    key: "sparkle",
    icon: iconForIntegrationPreset("sparkle"),
    labelKey: "github:presetIconSparkle",
  },
  { key: "check", icon: iconForIntegrationPreset("check"), labelKey: "github:presetIconCheck" },
];

export function iconForPresetKey(key: string | undefined): Icon {
  return iconForIntegrationPreset(key);
}

// Interpolate `{{url}}` and `{{title}}` placeholders in a prompt template.
// Also supports legacy single-brace `{url}` / `{title}` for backward compat.
// Unknown placeholders are left untouched so the user sees what's broken.
export function interpolatePromptTemplate(
  template: string,
  opts: { url: string; title: string },
): string {
  return template.replace(/\{\{?(url|title)\}\}?/g, (_match, key) => {
    if (key === "url") return opts.url;
    if (key === "title") return opts.title;
    return _match;
  });
}

export function toTaskPreset(stored: GitHubActionPreset): TaskPreset {
  return {
    id: stored.id,
    label: stored.label,
    hint: stored.hint,
    icon: iconForPresetKey(stored.icon),
    prompt: (opts) => interpolatePromptTemplate(stored.prompt_template, opts),
  };
}

// NOTE: `label`, `hint` and `prompt_template` below are NOT translated, and must
// not be. These records seed the editable draft in action-presets-section.tsx
// (`setPrDraft(DEFAULT_PR_PRESETS)`) and are persisted to workspace settings as
// `GitHubActionPreset`, so translating them would write locale-dependent values
// into a user's saved presets and leave them there after a locale switch.
// `prompt_template` is additionally sent to the agent verbatim. Localizing these
// needs a key/persisted-value split, the same open item as PR_PRESETS.
// i18n-exempt: persisted preset seed. Localizing needs a key/value split in the preset type.
export const DEFAULT_PR_PRESETS: GitHubActionPreset[] = [
  {
    id: "review",
    label: "Review",
    hint: "Read the diff, flag issues",
    icon: "eye",
    prompt_template:
      "Review the pull request at {{url}}. Provide feedback on code quality, correctness, and suggest improvements.",
  },
  {
    id: "address_feedback",
    label: "Address feedback",
    hint: "Apply review comments",
    icon: "message",
    prompt_template:
      "Review the feedback on the pull request at {{url}}. Evaluate each comment critically - apply changes that improve the code, push back on suggestions that are unnecessary or harmful, and explain your reasoning. Push the changes when done.",
  },
  {
    id: "fix_ci",
    label: "Fix CI",
    hint: "Diagnose failing checks",
    icon: "tool",
    prompt_template:
      "Investigate and fix the CI failures and merge conflicts on the pull request at {{url}}. Run the failing checks locally, resolve any conflicts, diagnose issues, and push fixes.",
  },
];

// i18n-exempt: persisted preset seed. Localizing needs a key/value split in the preset type.
export const DEFAULT_ISSUE_PRESETS: GitHubActionPreset[] = [
  {
    id: "implement",
    label: "Implement",
    hint: "Build and open a PR",
    icon: "code",
    prompt_template:
      'Implement the changes described in the GitHub issue at {{url}} (title: "{{title}}"). Open a pull request when complete.',
  },
  {
    id: "investigate",
    label: "Investigate",
    hint: "Find the root cause",
    icon: "search",
    prompt_template:
      'Investigate the GitHub issue at {{url}} (title: "{{title}}"). Identify root cause and summarize findings.',
  },
  {
    id: "reproduce",
    label: "Reproduce",
    hint: "Document repro steps",
    icon: "bug",
    prompt_template:
      'Reproduce the bug described in the GitHub issue at {{url}} (title: "{{title}}"). Document the reproduction steps.',
  },
];

// Resolve stored presets into runtime TaskPreset[]. Falls back to defaults while
// presets load so the /github page doesn't flicker empty dropdowns on first mount.
export function resolvePRPresets(stored: GitHubActionPresets | null): TaskPreset[] {
  const source = stored?.pr?.length ? stored.pr : DEFAULT_PR_PRESETS;
  return source.map(toTaskPreset);
}

export function resolveIssuePresets(stored: GitHubActionPresets | null): TaskPreset[] {
  const source = stored?.issue?.length ? stored.issue : DEFAULT_ISSUE_PRESETS;
  return source.map(toTaskPreset);
}
