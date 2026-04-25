import {
  IconEye,
  IconMessageDots,
  IconTool,
  IconCode,
  IconSearch,
  IconBug,
  IconSparkles,
  IconChecks,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";
import type {
  GitHubActionPreset,
  GitHubActionPresetIcon,
  GitHubActionPresets,
} from "@/lib/types/github";
import type { TaskPreset } from "./quick-task-launcher";

export const PRESET_ICON_CHOICES: { key: GitHubActionPresetIcon; icon: Icon; label: string }[] = [
  { key: "eye", icon: IconEye, label: "Eye" },
  { key: "message", icon: IconMessageDots, label: "Message" },
  { key: "tool", icon: IconTool, label: "Tool" },
  { key: "code", icon: IconCode, label: "Code" },
  { key: "search", icon: IconSearch, label: "Search" },
  { key: "bug", icon: IconBug, label: "Bug" },
  { key: "sparkle", icon: IconSparkles, label: "Sparkle" },
  { key: "check", icon: IconChecks, label: "Check" },
];

const ICON_BY_KEY: Record<string, Icon> = Object.fromEntries(
  PRESET_ICON_CHOICES.map((choice) => [choice.key, choice.icon]),
);

export function iconForPresetKey(key: string | undefined): Icon {
  if (!key) return IconSparkles;
  return ICON_BY_KEY[key] ?? IconSparkles;
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
      "Review the feedback on the pull request at {{url}}. Evaluate each comment critically — apply changes that improve the code, push back on suggestions that are unnecessary or harmful, and explain your reasoning. Push the changes when done.",
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
