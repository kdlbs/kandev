import { IconInbox, IconUserPlus, IconGitMerge, IconPlus } from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";

export type PresetGroup = "inbox" | "created";

export type PresetOption = {
  value: string;
  /** Catalog key; resolved where the preset is rendered. */
  labelKey: string;
  // Backend filter token consumed by gitlab.translateUserSearchFilter.
  // Accepted: "assigned_to_me", "created_by_me", "review_requested" (MRs only).
  // Everything else is treated as a raw `key=value&...` filter and is parsed
  // by appendFilter — keep this to the curated tokens; the custom-query input
  // is the escape hatch for everything else.
  filter: string;
  group: PresetGroup;
  icon: Icon;
};

/** Display label for a preset, or undefined when there is no preset. */
export function presetLabel(
  translate: (key: string) => string,
  preset: PresetOption | undefined,
): string | undefined {
  return preset ? translate(preset.labelKey) : undefined;
}

export const MR_PRESETS: PresetOption[] = [
  {
    value: "review_requested",
    labelKey: "gitlab:presetReviewRequested",
    filter: "review_requested",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "assigned",
    labelKey: "gitlab:presetAssigned",
    filter: "assigned_to_me",
    group: "inbox",
    icon: IconUserPlus,
  },
  {
    value: "authored",
    labelKey: "gitlab:presetAuthored",
    filter: "created_by_me",
    group: "created",
    icon: IconGitMerge,
  },
];

export const ISSUE_PRESETS: PresetOption[] = [
  {
    value: "assigned",
    labelKey: "gitlab:presetAssigned",
    filter: "assigned_to_me",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "created",
    labelKey: "gitlab:presetCreated",
    filter: "created_by_me",
    group: "created",
    icon: IconPlus,
  },
];
