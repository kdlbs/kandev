import type { AzureDevOpsActionPreset, AzureDevOpsQueryPreset } from "@/lib/types/azure-devops";

const WIQL_START = "SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = @project";
const WIQL_ORDER = " ORDER BY [System.ChangedDate] DESC";

function wiql(condition?: string): string {
  return `${WIQL_START}${condition ? ` AND ${condition}` : ""}${WIQL_ORDER}`;
}

export const DEFAULT_AZURE_WORK_ITEM_QUERIES: AzureDevOpsQueryPreset[] = [
  {
    id: "recent",
    label: "Recently updated",
    group: "inbox",
    filters: { wiql: wiql(), top: 50 },
  },
  {
    id: "assigned",
    label: "Assigned to me",
    group: "inbox",
    filters: { wiql: wiql("[System.AssignedTo] = @Me"), top: 50 },
  },
  {
    id: "active",
    label: "Active",
    group: "inbox",
    filters: {
      wiql: wiql("[System.State] <> 'Closed' AND [System.State] <> 'Done'"),
      top: 50,
    },
  },
  {
    id: "created",
    label: "Created by me",
    group: "created",
    filters: { wiql: wiql("[System.CreatedBy] = @Me"), top: 50 },
  },
];

export const DEFAULT_AZURE_PULL_REQUEST_QUERIES: AzureDevOpsQueryPreset[] = [
  {
    id: "review-requested",
    label: "Review requested",
    group: "inbox",
    filters: { status: "active", reviewer: "@me", creator: "" },
  },
  {
    id: "active",
    label: "Open",
    group: "inbox",
    filters: { status: "active", reviewer: "", creator: "" },
  },
  {
    id: "completed",
    label: "Completed",
    group: "created",
    filters: { status: "completed", reviewer: "", creator: "" },
  },
  {
    id: "created",
    label: "Created by me",
    group: "created",
    filters: { status: "active", reviewer: "", creator: "@me" },
  },
];

export const DEFAULT_AZURE_WORK_ITEM_ACTIONS: AzureDevOpsActionPreset[] = [
  {
    id: "implement",
    label: "Implement",
    hint: "Build and open a PR",
    icon: "code",
    promptTemplate:
      'Implement the Azure DevOps work item at {{url}} (title: "{{title}}"). Open a pull request when complete.',
  },
  {
    id: "investigate",
    label: "Investigate",
    hint: "Find the root cause",
    icon: "search",
    promptTemplate:
      'Investigate the Azure DevOps work item at {{url}} (title: "{{title}}"). Identify the root cause and summarize findings.',
  },
  {
    id: "reproduce",
    label: "Reproduce",
    hint: "Document repro steps",
    icon: "bug",
    promptTemplate:
      'Reproduce the Azure DevOps work item at {{url}} (title: "{{title}}"). Document the reproduction steps.',
  },
];

export const DEFAULT_AZURE_PULL_REQUEST_ACTIONS: AzureDevOpsActionPreset[] = [
  {
    id: "review",
    label: "Review",
    hint: "Read the diff, flag issues",
    icon: "eye",
    promptTemplate:
      "Review the Azure DevOps pull request at {{url}}. Provide feedback on code quality and correctness.",
  },
  {
    id: "address-feedback",
    label: "Address feedback",
    hint: "Apply review comments",
    icon: "message",
    promptTemplate: "Review and address the feedback on the Azure DevOps pull request at {{url}}.",
  },
  {
    id: "fix-ci",
    label: "Fix CI",
    hint: "Diagnose failing checks",
    icon: "tool",
    promptTemplate: "Investigate and fix CI failures for the Azure DevOps pull request at {{url}}.",
  },
];
