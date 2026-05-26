import { IconInbox, IconUserPlus, IconGitMerge, IconPlus } from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";

export type PresetGroup = "inbox" | "created";

export type PresetOption = {
  value: string;
  label: string;
  // Backend filter token consumed by gitlab.translateUserSearchFilter.
  // Accepted: "assigned_to_me", "created_by_me", "review_requested" (MRs only).
  // Everything else is treated as a raw `key=value&...` filter and is parsed
  // by appendFilter — keep this to the curated tokens; the custom-query input
  // is the escape hatch for everything else.
  filter: string;
  group: PresetGroup;
  icon: Icon;
};

export const MR_PRESETS: PresetOption[] = [
  {
    value: "review_requested",
    label: "Review requested",
    filter: "review_requested",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "assigned",
    label: "Assigned",
    filter: "assigned_to_me",
    group: "inbox",
    icon: IconUserPlus,
  },
  {
    value: "authored",
    label: "Authored",
    filter: "created_by_me",
    group: "created",
    icon: IconGitMerge,
  },
];

export const ISSUE_PRESETS: PresetOption[] = [
  {
    value: "assigned",
    label: "Assigned",
    filter: "assigned_to_me",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "created",
    label: "Created",
    filter: "created_by_me",
    group: "created",
    icon: IconPlus,
  },
];
