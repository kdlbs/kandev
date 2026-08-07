import type { Icon } from "@tabler/icons-react";
import {
  IconBug,
  IconChecks,
  IconCode,
  IconEye,
  IconMessageDots,
  IconSearch,
  IconSparkles,
  IconTool,
} from "@tabler/icons-react";

// Runtime shape consumed by the dropdown menus. `prompt` is a resolver — for
// stored presets it interpolates the template; for fallback/defaults it's a
// plain string-builder.
export type JiraTaskPreset = {
  id: string;
  label: string;
  hint: string;
  icon: Icon;
  prompt: (opts: { url: string; key: string; title: string; description: string }) => string;
};

export type JiraPresetIcon =
  | "code"
  | "search"
  | "eye"
  | "message"
  | "tool"
  | "bug"
  | "sparkle"
  | "check";

// Persisted shape stored in backend user settings and edited from settings.
export type JiraStoredPreset = {
  id: string;
  label: string;
  hint: string;
  icon: JiraPresetIcon | (string & {});
  prompt_template: string;
};

// `key` is the persisted `JiraPresetIcon` enum; only the label is copy, so it
// travels as a catalog key resolved at render (module-scope `t()` would freeze
// at the boot locale — see docs/i18n.md). Same shape as
// `PRESET_ICON_CHOICES` in components/github/my-github/action-presets.ts.
export const PRESET_ICON_CHOICES: { key: JiraPresetIcon; icon: Icon; labelKey: string }[] = [
  { key: "code", icon: IconCode, labelKey: "jira:presetIconCode" },
  { key: "search", icon: IconSearch, labelKey: "jira:presetIconSearch" },
  { key: "eye", icon: IconEye, labelKey: "jira:presetIconEye" },
  { key: "message", icon: IconMessageDots, labelKey: "jira:presetIconMessage" },
  { key: "tool", icon: IconTool, labelKey: "jira:presetIconTool" },
  { key: "bug", icon: IconBug, labelKey: "jira:presetIconBug" },
  { key: "sparkle", icon: IconSparkles, labelKey: "jira:presetIconSparkle" },
  { key: "check", icon: IconChecks, labelKey: "jira:presetIconCheck" },
];

const ICON_BY_KEY: Record<string, Icon> = Object.fromEntries(
  PRESET_ICON_CHOICES.map((c) => [c.key, c.icon]),
);

export function iconForPresetKey(key: string | undefined): Icon {
  if (!key) return IconSparkles;
  return ICON_BY_KEY[key] ?? IconSparkles;
}

// Interpolate `{{url}}`, `{{key}}`, `{{title}}`, `{{description}}` placeholders.
// Also supports single-brace `{foo}` for convenience. Empty description falls
// back to "(no description)" so prompts read naturally.
//
// That fallback is NOT translated: it is substituted into a prompt sent to the
// agent, not rendered as UI copy, and the agent reads the whole template as one
// instruction in whatever language the user wrote it.
export function interpolateJiraTemplate(
  template: string,
  opts: { url: string; key: string; title: string; description: string },
): string {
  const description = opts.description || "(no description)";
  return template.replace(/\{\{?(url|key|title|description)\}\}?/g, (_m, k: string) => {
    switch (k) {
      case "url":
        return opts.url;
      case "key":
        return opts.key;
      case "title":
        return opts.title;
      case "description":
        return description;
      default:
        return _m;
    }
  });
}

export function toTaskPreset(stored: JiraStoredPreset): JiraTaskPreset {
  return {
    id: stored.id,
    label: stored.label,
    hint: stored.hint,
    icon: iconForPresetKey(stored.icon),
    prompt: (opts) => interpolateJiraTemplate(stored.prompt_template, opts),
  };
}

// NOTE: `label`, `hint` and `prompt_template` below are NOT translated, and must
// not be. These seed the user's editable preset list and are PERSISTED verbatim
// into user settings on the first save, so a locale-dependent value would be
// written into the user's data and stay there after a locale switch. The prompt
// templates are also sent to the agent as instructions rather than shown as copy.
export const DEFAULT_JIRA_PRESETS: JiraStoredPreset[] = [
  {
    id: "implement",
    label: "Implement",
    hint: "Build the change, open a PR",
    icon: "code",
    prompt_template:
      "Implement the change described in Jira ticket {{key}} ({{url}}).\n\nSummary: {{title}}\n\nDescription:\n{{description}}\n\nWhen done, open a pull request and link it back to {{key}}.",
  },
  {
    id: "investigate",
    label: "Investigate",
    hint: "Find the root cause",
    icon: "search",
    prompt_template:
      "Investigate Jira ticket {{key}} ({{url}}).\n\nSummary: {{title}}\n\nDescription:\n{{description}}\n\nIdentify the root cause and summarise findings; do not make code changes unless asked.",
  },
];

export function resolveJiraTaskPresets(stored: JiraStoredPreset[] | null): JiraTaskPreset[] {
  const source = stored?.length ? stored : DEFAULT_JIRA_PRESETS;
  return source.map(toTaskPreset);
}
