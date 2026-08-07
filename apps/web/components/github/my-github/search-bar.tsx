import {
  IconInbox,
  IconAt,
  IconGitPullRequest,
  IconPencil,
  IconGitMerge,
  IconPlus,
  IconCircleCheck,
} from "@tabler/icons-react";
import type { Icon } from "@tabler/icons-react";

export type PresetGroup = "inbox" | "created";

export type PresetOption = {
  value: string;
  label: string;
  filter: string;
  group: PresetGroup;
  icon: Icon;
};

// NOTE: `label` here is NOT translated, and must not be. `toStored()` in
// use-default-query-presets.ts copies it verbatim into `StoredQueryPreset`,
// which is persisted to `github_default_query_presets` in user and workspace
// settings and is user-editable in Settings → Integrations → GitHub. Translating
// it in place would write locale-dependent values into saved settings and leave
// them there after a locale switch. Localizing these needs a key/persisted-value
// split in this module, the same open item as the built-in layout profile names.
// `value`, `filter` and `group` are wire values and a GitHub search query.
export const PR_PRESETS: PresetOption[] = [
  {
    value: "review_requested",
    label: "Review requested",
    filter: "review-requested:@me is:open",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "mentioned",
    label: "Mentions",
    filter: "mentions:@me is:open",
    group: "inbox",
    icon: IconAt,
  },
  {
    value: "open",
    label: "Open",
    filter: "author:@me is:open",
    group: "created",
    icon: IconGitPullRequest,
  },
  {
    value: "drafts",
    label: "Drafts",
    filter: "author:@me is:open draft:true",
    group: "created",
    icon: IconPencil,
  },
  {
    value: "merged",
    label: "Recently merged",
    filter: "author:@me is:merged",
    group: "created",
    icon: IconGitMerge,
  },
];

export const ISSUE_PRESETS: PresetOption[] = [
  {
    value: "assigned",
    label: "Assigned",
    filter: "assignee:@me is:open",
    group: "inbox",
    icon: IconInbox,
  },
  {
    value: "mentioned",
    label: "Mentions",
    filter: "mentions:@me is:open",
    group: "inbox",
    icon: IconAt,
  },
  {
    value: "created",
    label: "Open",
    filter: "author:@me is:open",
    group: "created",
    icon: IconPlus,
  },
  {
    value: "closed",
    label: "Recently closed",
    filter: "author:@me is:closed",
    group: "created",
    icon: IconCircleCheck,
  },
];
