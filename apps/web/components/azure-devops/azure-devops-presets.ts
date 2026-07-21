import {
  IconActivity,
  IconCalendarClock,
  IconChecks,
  IconGitPullRequest,
  IconInbox,
  IconUser,
} from "@tabler/icons-react";
import type { AzureDevOpsFiltersState } from "./azure-devops-filters";

export type AzureDevOpsPresetKind = "work_item" | "pull_request";

export type AzureDevOpsPreset = {
  value: string;
  label: string;
  icon: typeof IconInbox;
  group: "inbox" | "created";
  filters: Partial<AzureDevOpsFiltersState>;
};

const WIQL_START = "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project";
const WIQL_ORDER = " ORDER BY [System.ChangedDate] DESC";

function wiql(condition?: string): string {
  return `${WIQL_START}${condition ? ` AND ${condition}` : ""}${WIQL_ORDER}`;
}

export const AZURE_WORK_ITEM_PRESETS: AzureDevOpsPreset[] = [
  {
    value: "recent",
    label: "Recently updated",
    icon: IconCalendarClock,
    group: "inbox",
    filters: { wiql: wiql(), top: 50 },
  },
  {
    value: "assigned",
    label: "Assigned to me",
    icon: IconInbox,
    group: "inbox",
    filters: { wiql: wiql("[System.AssignedTo] = @Me"), top: 50 },
  },
  {
    value: "active",
    label: "Active",
    icon: IconActivity,
    group: "inbox",
    filters: { wiql: wiql("[System.State] <> 'Closed' AND [System.State] <> 'Done'"), top: 50 },
  },
  {
    value: "created",
    label: "Created by me",
    icon: IconUser,
    group: "created",
    filters: { wiql: wiql("[System.CreatedBy] = @Me"), top: 50 },
  },
];

export const AZURE_PULL_REQUEST_PRESETS: AzureDevOpsPreset[] = [
  {
    value: "review-requested",
    label: "Review requested",
    icon: IconInbox,
    group: "inbox",
    filters: { status: "active", reviewer: "@me", creator: "" },
  },
  {
    value: "active",
    label: "Open",
    icon: IconGitPullRequest,
    group: "inbox",
    filters: { status: "active", creator: "", reviewer: "" },
  },
  {
    value: "completed",
    label: "Completed",
    icon: IconChecks,
    group: "created",
    filters: { status: "completed", creator: "", reviewer: "" },
  },
  {
    value: "created",
    label: "Created by me",
    icon: IconUser,
    group: "created",
    filters: { status: "active", creator: "@me", reviewer: "" },
  },
];

export function presetsForKind(kind: AzureDevOpsPresetKind): AzureDevOpsPreset[] {
  return kind === "work_item" ? AZURE_WORK_ITEM_PRESETS : AZURE_PULL_REQUEST_PRESETS;
}
