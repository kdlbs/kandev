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

// Persisted shape — stored in localStorage and edited from settings.
export type JiraStoredPreset = {
  id: string;
  label: string;
  hint: string;
  icon: JiraPresetIcon | (string & {});
  prompt_template: string;
};

export const PRESET_ICON_CHOICES: { key: JiraPresetIcon; icon: Icon; label: string }[] = [
  { key: "code", icon: IconCode, label: "Code" },
  { key: "search", icon: IconSearch, label: "Search" },
  { key: "eye", icon: IconEye, label: "Eye" },
  { key: "message", icon: IconMessageDots, label: "Message" },
  { key: "tool", icon: IconTool, label: "Tool" },
  { key: "bug", icon: IconBug, label: "Bug" },
  { key: "sparkle", icon: IconSparkles, label: "Sparkle" },
  { key: "check", icon: IconChecks, label: "Check" },
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
