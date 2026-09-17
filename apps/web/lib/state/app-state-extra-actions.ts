import type { UISliceActions as UIA } from "./slices/ui/types";
import type {
  SystemSliceActions,
  AutomationsSliceActions,
  FeaturesSliceActions,
  AuthSliceActions,
  GitHubSliceActions,
  GitLabSliceActions,
  AzureDevOpsSliceActions,
  JiraSliceActions,
  LinearSliceActions,
  OfficeSliceActions,
  PluginsSliceActions,
  ReviewSliceActions,
  NeedsYouInboxSliceActions,
  FailedInboxSliceActions,
} from "./slices";

// Split out of app-state-types.ts to stay under the file line limit. These
// slices contribute actions only (no AppState fields declared here).
export type AppStateExtraActions = Pick<UIA, "setThreadActiveView" | "createThreadView"> &
  GitHubSliceActions &
  GitLabSliceActions &
  JiraSliceActions &
  LinearSliceActions &
  OfficeSliceActions &
  import("./store-reexports").WorkspaceSourceStoreState &
  AzureDevOpsSliceActions &
  SystemSliceActions &
  FeaturesSliceActions &
  AuthSliceActions &
  AutomationsSliceActions &
  PluginsSliceActions &
  ReviewSliceActions &
  NeedsYouInboxSliceActions &
  FailedInboxSliceActions;
