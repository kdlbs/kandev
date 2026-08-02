import {
  IconBug,
  IconChecks,
  IconCode,
  IconEye,
  IconMessageDots,
  IconSearch,
  IconSparkles,
  IconTool,
  type Icon,
} from "@tabler/icons-react";

// `key` is the persisted icon enum stored on an action preset; only the label is
// copy, so it travels as a catalog key resolved at render (a module-scope `t()`
// would freeze at the boot locale — see docs/i18n.md). The keys live in
// `common` because this catalog is integration-agnostic; the GitHub and Jira
// copies of this shape predate it and keep their own namespaced keys.
export const ACTION_PRESET_ICON_CHOICES: Array<{ key: string; icon: Icon; labelKey: string }> = [
  { key: "eye", icon: IconEye, labelKey: "common:presetIconEye" },
  { key: "message", icon: IconMessageDots, labelKey: "common:presetIconMessage" },
  { key: "tool", icon: IconTool, labelKey: "common:presetIconTool" },
  { key: "code", icon: IconCode, labelKey: "common:presetIconCode" },
  { key: "search", icon: IconSearch, labelKey: "common:presetIconSearch" },
  { key: "bug", icon: IconBug, labelKey: "common:presetIconBug" },
  { key: "sparkle", icon: IconSparkles, labelKey: "common:presetIconSparkle" },
  { key: "check", icon: IconChecks, labelKey: "common:presetIconCheck" },
];

const ICONS = new Map(ACTION_PRESET_ICON_CHOICES.map((choice) => [choice.key, choice.icon]));

export function iconForActionPreset(key: string | undefined): Icon {
  return (key && ICONS.get(key)) || IconSparkles;
}
