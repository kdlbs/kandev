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

export const ACTION_PRESET_ICON_CHOICES: Array<{ key: string; icon: Icon; label: string }> = [
  { key: "eye", icon: IconEye, label: "Eye" },
  { key: "message", icon: IconMessageDots, label: "Message" },
  { key: "tool", icon: IconTool, label: "Tool" },
  { key: "code", icon: IconCode, label: "Code" },
  { key: "search", icon: IconSearch, label: "Search" },
  { key: "bug", icon: IconBug, label: "Bug" },
  { key: "sparkle", icon: IconSparkles, label: "Sparkle" },
  { key: "check", icon: IconChecks, label: "Check" },
];

const ICONS = new Map(ACTION_PRESET_ICON_CHOICES.map((choice) => [choice.key, choice.icon]));

export function iconForActionPreset(key: string | undefined): Icon {
  return (key && ICONS.get(key)) || IconSparkles;
}
