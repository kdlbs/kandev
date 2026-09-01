/**
 * Public, runtime-free frontend contract for Kandev plugins.
 *
 * This package deliberately has no imports from React, the Kandev web
 * application, Zustand, or host UI implementation modules. Plugins receive
 * all runtime values during initialize and use these structural types at
 * compile time only.
 */
export type HostNode = unknown;
// `any` is deliberate at these two React compatibility seams. Reproducing
// React's overloaded createElement and recursive ReactNode types would make
// this runtime-free package depend on React's private type graph again.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type ElementFactory = (...args: any[]) => HostNode;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type Component<Props = {}> = (props: Props) => any;
export type HostComponent = unknown;

export interface PluginIconProps {
  className?: string;
  "aria-hidden"?: boolean | "true" | "false";
}

/** Curated host icon name or a plugin-owned component rendered with host React. */
export type PluginIcon = string | Component<PluginIconProps>;

/** Placement for a registered nav item; see `PluginRegistry.registerNavItem`. */
export type PluginNavSection = "main" | "settings" | "integrations" | "sidebar-footer";

/** Context passed to components registered for the `main-top-bar` slot. */
export interface MainTopBarSlotProps {
  workspaceId: string | null;
  workspaceLabel?: string;
  currentPage: "kanban" | "tasks";
  presentation: "desktop" | "mobile";
}

export type StateUpdater<Value> = Value | ((previous: Value) => Value);
export type StateSetter<Value> = (value: StateUpdater<Value>) => void;

export interface MutableRef<Value> {
  current: Value;
}

export interface ResponsiveBreakpoint {
  isMobile: boolean;
  usesDesktopWorkbench?: boolean;
}

/** React-compatible primitives supplied by the host; no React package is required to consume them. */
export interface HostReact {
  readonly Fragment: HostComponent;
  createElement: ElementFactory;
  useState<Value>(initialValue: Value | (() => Value)): [Value, StateSetter<Value>];
  useEffect(effect: () => void | (() => void), dependencies?: readonly unknown[]): void;
  useMemo<Value>(factory: () => Value, dependencies: readonly unknown[]): Value;
  useCallback<Callback extends (...args: never[]) => unknown>(
    callback: Callback,
    dependencies: readonly unknown[],
  ): Callback;
  useRef<Value>(initialValue: Value): MutableRef<Value>;
}

export interface ActionInput {
  workspaceId?: string;
  taskId?: string;
  sessionId?: string;
  repositoryId?: string;
  body?: unknown;
}

export interface PluginHostRepository {
  id: string;
  workspace_id: string;
  name: string;
  provider: string;
  source_type?: string;
  provider_repo_id?: string;
  provider_host?: string;
  provider_scope?: string;
  provider_owner?: string;
  provider_name?: string;
  remote_url?: string;
  default_branch?: string;
}

export interface TaskCreationStep {
  id: string;
  title: string;
  events?: Record<string, unknown>;
}

export interface TaskCreationContext {
  workspaceId: string;
  workflowId: string;
  defaultStepId: string;
  steps: readonly TaskCreationStep[];
  repositories: readonly PluginHostRepository[];
}

export interface RepositoryIdentityInput {
  workspaceId: string;
  providerId: string;
  providerScope: string;
  providerRepositoryId: string;
}

/** Provider-neutral reads that replace knowledge of Kandev's private app store. */
export interface PluginContextApi {
  getActiveWorkspaceId(): string | undefined;
  subscribeActiveWorkspace(listener: (workspaceId: string | undefined) => void): () => void;
  /** Returns the ids of all workspaces currently available to the user. */
  getWorkspaceIds(): readonly string[];
  /** Notifies the plugin when the available workspace ids change. */
  subscribeWorkspaces(listener: (workspaceIds: readonly string[]) => void): () => void;
  getTaskCreationContext(workspaceId: string): TaskCreationContext | null;
  subscribeTaskCreationContext(
    workspaceId: string,
    listener: (context: TaskCreationContext | null) => void,
  ): () => void;
  resolveRepositoryId(identity: RepositoryIdentityInput): string | undefined;
}

export interface RepositoryInspection {
  providerId: string;
  providerHost: string;
  providerScope?: string;
  ownerOrProject: string;
  repositoryId: string;
  repositoryName: string;
  cloneUrl: string;
  defaultBranch?: string;
  baseBranch?: string;
  headBranch?: string;
  pullRequest?: { number: number; title: string };
}

export interface RepositoryProviderRegistration {
  id: string;
  label: string;
  icon?: PluginIcon;
  listRepositories(context: {
    workspaceId: string;
    query?: string;
    cursor?: string;
    limit?: number;
    signal: AbortSignal;
  }): Promise<
    RepositoryInspection[] | { repositories: RepositoryInspection[]; nextCursor?: string }
  >;
  /** Optional cheap hint only; inspectURL is the ownership authority. */
  matchesURL?(url: string): boolean;
  listBranches(context: {
    workspaceId: string;
    repository: RepositoryInspection;
    signal: AbortSignal;
  }): Promise<Array<{ name: string }>>;
  inspectURL(context: {
    workspaceId: string;
    url: string;
    signal: AbortSignal;
  }): Promise<RepositoryInspection | null>;
  supportsDraft?: boolean;
  createChangeRequest?(context: {
    workspaceId: string;
    taskId: string;
    sessionId: string;
    repositoryId: string;
    repository: PluginHostRepository;
    title: string;
    body: string;
    baseBranch?: string;
    draft: boolean;
    signal: AbortSignal;
  }): Promise<{
    url: string;
    provider?: string;
    output?: string;
    linked?: boolean;
    associationError?: string;
  }>;
}

export interface TaskContext {
  workspaceId: string;
  taskId: string;
  repositories: readonly PluginHostRepository[];
  pathname: string;
  presentation: "desktop" | "mobile";
}

export type ReviewTaskPipelineState = "success" | "failure" | "pending" | "neutral";

export interface ReviewTaskStatusCheck {
  id: string;
  label: string;
  state: ReviewTaskPipelineState;
  detail?: string;
  url?: string;
}

export interface ReviewTaskStatus {
  number: number | string;
  state: "open" | "merged" | "closed" | "draft";
  pipelineState: ReviewTaskPipelineState;
  checks: readonly ReviewTaskStatusCheck[];
  review?: {
    state: "approved" | "changes_requested" | "pending";
    approved: number;
    required?: number;
    requested?: number;
  };
  unresolvedComments?: number;
  loading?: boolean;
  error?: string;
  updatedAt?: number;
}

export interface ReviewSummary {
  providerId: string;
  reviewKey: string;
  title: string;
  url: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
  state: string;
  statusBadge?: { label: string; tone?: string };
  taskStatus?: ReviewTaskStatus;
}

export interface ReviewTaskAssociation {
  providerId: string;
  taskId: string;
  reviewKey: string;
  connectionScope: string;
  repositoryId: string;
  changeRequestNumber: string | number;
}

export interface QueryState<T> {
  data: T | null;
  loading: boolean;
  error: string | null;
  lastFetchedAt: Date | null;
  refresh(): void;
}

export interface PluginPageChrome {
  title?: string;
  subtitle?: string;
  icon?: PluginIcon;
  backHref?: string;
  backLabel?: string;
  actions?: Component;
}

export interface PluginRouteOptions {
  topbar?: boolean | PluginPageChrome;
}

export interface PluginTaskPanelProps {
  panelId: string;
  taskId: string;
  sessionId: string | null;
  presentation: "desktop" | "mobile";
}

export interface TaskPanelRegistration {
  id: string;
  title: string;
  icon?: PluginIcon;
  Component: Component<PluginTaskPanelProps>;
  mobileEnabled?: boolean;
}

export interface PluginTaskMenuContext {
  workspaceId: string;
  taskId: string;
  taskTitle: string;
  workflowStepId: string | null;
  presentation: "desktop" | "mobile";
}

export interface TaskMenuActionRegistration {
  id: string;
  label: string;
  icon?: PluginIcon;
  group: "edit" | "primary";
  visible?(context: PluginTaskMenuContext): boolean;
  run(context: PluginTaskMenuContext): void | Promise<void>;
}

export interface PluginTaskFilterOption {
  value: string;
  label: string;
  color?: string;
}

export interface TaskFilterRegistration {
  id: string;
  label: string;
  getOptions(): PluginTaskFilterOption[];
  matches(context: { taskId: string }, selected: string[]): boolean;
}

export interface TaskListFacetValue {
  value: string;
  label: string;
  color?: string;
}

/** A synchronous, page-local facet contribution for the host task list. */
export interface TaskListFacetRegistration {
  id: string;
  label: string;
  getValues(context: { taskId: string; workspaceId?: string }): readonly TaskListFacetValue[];
  subscribe?(listener: () => void): () => void;
}

export type PluginStorageScope = "instance" | "workspace" | "task" | "session" | "repository";

export interface PluginStorageEntry {
  key: string;
  value: unknown;
  updatedAt: string;
}

export interface PluginUserStateChange {
  scope: PluginStorageScope;
  scopeId: string;
  key: string;
  updatedAt: string;
  deleted?: boolean;
}

export interface PluginStorageApi {
  get(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    options?: { signal?: AbortSignal },
  ): Promise<PluginStorageEntry | undefined>;
  set(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    value: unknown,
    options?: { signal?: AbortSignal; writerId?: string; ifUnmodifiedSince?: string },
  ): Promise<{ updatedAt: string }>;
  delete(
    scope: PluginStorageScope,
    scopeId: string,
    key: string,
    options?: { signal?: AbortSignal; writerId?: string },
  ): Promise<void>;
  list(
    scope: PluginStorageScope,
    scopeId: string,
    options?: { signal?: AbortSignal },
  ): Promise<PluginStorageEntry[]>;
  subscribe(
    filter: { scope?: PluginStorageScope; scopeId?: string; key?: string; writerId?: string },
    listener: (change: PluginUserStateChange) => void,
  ): () => void;
}

/** Named host components. Values are runtime-owned and rendered through host.jsx. */
interface PluginUIShape {
  Accordion: unknown;
  AccordionContent: unknown;
  AccordionItem: unknown;
  AccordionTrigger: unknown;
  Alert: unknown;
  AlertDescription: unknown;
  AlertTitle: unknown;
  Badge: unknown;
  Button: unknown;
  Card: unknown;
  CardAction: unknown;
  CardContent: unknown;
  CardDescription: unknown;
  CardFooter: unknown;
  CardHeader: unknown;
  CardTitle: unknown;
  ChartContainer: unknown;
  ChartLegend: unknown;
  ChartLegendContent: unknown;
  ChartStyle: unknown;
  ChartTooltip: unknown;
  ChartTooltipContent: unknown;
  Checkbox: unknown;
  Collapsible: unknown;
  CollapsibleContent: unknown;
  CollapsibleTrigger: unknown;
  ChangeRequestDetail: unknown;
  ChangeRequestList: unknown;
  ChangeRequestRow: unknown;
  Dialog: unknown;
  DialogClose: unknown;
  DialogContent: unknown;
  DialogDescription: unknown;
  DialogFooter: unknown;
  DialogHeader: unknown;
  DialogTitle: unknown;
  DialogTrigger: unknown;
  Drawer: unknown;
  DrawerClose: unknown;
  DrawerContent: unknown;
  DrawerDescription: unknown;
  DrawerFooter: unknown;
  DrawerHeader: unknown;
  DrawerOverlay: unknown;
  DrawerPortal: unknown;
  DrawerTitle: unknown;
  DrawerTrigger: unknown;
  DropdownMenu: unknown;
  DropdownMenuContent: unknown;
  DropdownMenuItem: unknown;
  DropdownMenuSeparator: unknown;
  DropdownMenuTrigger: unknown;
  Empty: unknown;
  EmptyContent: unknown;
  EmptyDescription: unknown;
  EmptyHeader: unknown;
  EmptyMedia: unknown;
  EmptyTitle: unknown;
  Input: unknown;
  IntegrationCursorPagination: unknown;
  IntegrationChangeRequestStatus: unknown;
  IntegrationIcon: unknown;
  IntegrationListToolbar: unknown;
  IntegrationRepositoryFilter: unknown;
  IntegrationSaveQueryDialog: unknown;
  IntegrationScopeBar: unknown;
  IntegrationStartTaskMenu: unknown;
  Kbd: unknown;
  KbdGroup: unknown;
  Label: unknown;
  PageTopbar: unknown;
  Pagination: unknown;
  PaginationContent: unknown;
  PaginationEllipsis: unknown;
  PaginationItem: unknown;
  PaginationLink: unknown;
  PaginationNext: unknown;
  PaginationPrevious: unknown;
  Popover: unknown;
  PopoverAnchor: unknown;
  PopoverContent: unknown;
  PopoverDescription: unknown;
  PopoverHeader: unknown;
  PopoverTitle: unknown;
  PopoverTrigger: unknown;
  Progress: unknown;
  RichTextEditor: unknown;
  RichTextReadOnly: unknown;
  ScrollArea: unknown;
  Select: unknown;
  SelectContent: unknown;
  SelectItem: unknown;
  SelectTrigger: unknown;
  SelectValue: unknown;
  Separator: unknown;
  Sheet: unknown;
  SheetClose: unknown;
  SheetContent: unknown;
  SheetDescription: unknown;
  SheetFooter: unknown;
  SheetHeader: unknown;
  SheetTitle: unknown;
  SheetTrigger: unknown;
  Skeleton: unknown;
  Spinner: unknown;
  Switch: unknown;
  Table: unknown;
  TableBody: unknown;
  TableCell: unknown;
  TableHead: unknown;
  TableHeader: unknown;
  TableRow: unknown;
  Tabs: unknown;
  TabsContent: unknown;
  TabsList: unknown;
  TabsTrigger: unknown;
  TaskCreateDialog: unknown;
  TaskRowIndicator: unknown;
  Textarea: unknown;
  Tooltip: unknown;
  TooltipContent: unknown;
  TooltipProvider: unknown;
  TooltipTrigger: unknown;
  Combobox: unknown;
  IntegrationAuthStatusBanner: unknown;
  IntegrationEnabledControl: unknown;
  SettingsSection: unknown;
  SettingsCard: unknown;
  WorkspaceScopedSection: unknown;
}

export type SettingsSaveRevision = string | number;

export type SettingsSaveContributor = {
  id: string;
  order?: number;
  revision: SettingsSaveRevision;
  isDirty: boolean;
  canSave?: boolean;
  invalidReason?: string;
  save: (revision: SettingsSaveRevision) => Promise<void> | void;
  discard: (revision?: SettingsSaveRevision) => Promise<void> | void;
};

export type PluginUIApi = {
  readonly [Name in keyof PluginUIShape]: HostComponent;
};

export interface PluginToastApi {
  (message: string, options?: Record<string, unknown>): string | number;
  success(message: string, options?: Record<string, unknown>): string | number;
  error(message: string, options?: Record<string, unknown>): string | number;
  warning(message: string, options?: Record<string, unknown>): string | number;
  info(message: string, options?: Record<string, unknown>): string | number;
  dismiss(id?: string | number): unknown;
}

export type PluginTranslationValues = Readonly<Record<string, string | number>>;

export type PluginTranslationOptions = {
  defaultValue?: string;
  count?: number;
  values?: PluginTranslationValues;
};

export type PluginTranslationCatalogs = Readonly<Record<string, Readonly<Record<string, string>>>>;

export interface PluginI18nApi {
  readonly locale: string;
  t(key: string, options?: PluginTranslationOptions): string;
  useTranslation(): {
    readonly locale: string;
    t(key: string, options?: PluginTranslationOptions): string;
  };
}

export interface PluginHostApi {
  pluginId: string;
  React: HostReact;
  jsx: ElementFactory;
  ui: PluginUIApi;
  i18n: PluginI18nApi;
  context: PluginContextApi;
  api: {
    readonly baseUrl: string;
    fetch(path: string, init?: RequestInit): Promise<Response>;
    invokeAction<T>(
      key: string,
      input?: ActionInput,
      options?: { signal?: AbortSignal },
    ): Promise<T>;
  };
  useResponsiveBreakpoint(): ResponsiveBreakpoint;
  readonly theme: "light" | "dark";
  onThemeChange(listener: (theme: "light" | "dark") => void): () => void;
  navigate(href: string, options?: { replace?: boolean }): void;
  openModal(options: {
    title?: string;
    description?: string;
    content: Component;
    size?: "sm" | "md" | "lg" | "xl";
    presentation?: "dialog" | "drawer";
    dismissible?: boolean;
  }): { close(): void };
  openTaskLinkDialog(options: {
    title: string;
    description: string;
    inputLabel: string;
    placeholder?: string;
    emptyError: string;
    failureMessage: string;
    successMessage: string;
    inputTestId?: string;
    errorTestId?: string;
    submitTestId?: string;
    onSubmit(reference: string, signal: AbortSignal): Promise<void>;
  }): { close(): void };
  openTaskReview(
    options:
      | {
          providerId: string;
          reviewKey: string;
          connectionScope: string;
          repositoryId: string;
          changeRequestNumber: string | number;
          title: string;
          presentation: "desktop";
          sessionId?: string;
        }
      | {
          providerId: string;
          reviewKey: string;
          connectionScope: string;
          repositoryId: string;
          changeRequestNumber: string | number;
          title: string;
          presentation: "mobile";
          sessionId: string;
        },
  ): void;
  toast: PluginToastApi;
  utils: {
    cn(...inputs: unknown[]): string;
    generateUUID(): string;
    formatRelativeTime(value: string | number | Date): string;
    integrationStatusRefreshMs: number;
  };
  useSettingsSaveContributor(contributor: SettingsSaveContributor): void;
  /**
   * Publishes this plugin integration's enabled state for one workspace.
   * Drives the host sidebar's "Enabled" badge (per workspace) reactively;
   * persist the durable value yourself (e.g. `host.storage`) — this call
   * only updates live UI state.
   */
  setIntegrationEnabled(integrationId: string, workspaceId: string, enabled: boolean): void;
  storage: PluginStorageApi;
}

export type IntegrationSettingsActionSurface = "detail" | "index";

export interface IntegrationSettingsActionProps {
  workspaceId?: string;
  /** Identifies the native integration surface that mounted the action. */
  surface: IntegrationSettingsActionSurface;
}

export interface PluginRegistry {
  registerTranslations(catalogs: PluginTranslationCatalogs): void;
  registerRoute(path: string, component: Component, options?: PluginRouteOptions): void;
  registerNavItem(item: {
    id: string;
    label: string;
    path: string;
    icon?: PluginIcon;
    section?: PluginNavSection;
  }): void;
  registerSettingsRoute(path: string, component: Component): void;
  registerComponent(slot: string, component: Component<{ slotProps?: unknown }>): void;
  registerWsHandler(action: string, handler: (payload: unknown) => void): void;
  registerKeybinding(id: string, handler: (event: KeyboardEvent) => void): void;
  registerIntegrationSettings(settings: {
    id: string;
    label: string;
    description: string;
    icon?: PluginIcon;
    Component: Component<{ workspaceId?: string }>;
    /** Optional action rendered in the detail section header and index card.
     * Receives the routed workspace and the native surface that mounted it. */
    action?: Component<IntegrationSettingsActionProps>;
  }): void;
  registerRepositoryProvider(provider: RepositoryProviderRegistration): void;
  registerTaskAction(action: {
    id: string;
    label: string;
    icon?: PluginIcon;
    placement: "link";
    group?: string;
    visible?(context: TaskContext): boolean;
    singleTaskOnly?: boolean;
    run(context: TaskContext): Promise<void>;
  }): void;
  registerReviewProvider(provider: {
    id: string;
    label: string;
    icon?: PluginIcon;
    changeRequestNoun: string;
    order: number;
    getSnapshot(taskId: string): readonly ReviewSummary[];
    subscribe(taskId: string, listener: () => void): () => void;
    refresh(taskId: string, signal: AbortSignal): Promise<void>;
    getAssociationSnapshot?(workspaceId: string): readonly ReviewTaskAssociation[];
    subscribeAssociations?(workspaceId: string, listener: () => void): () => void;
    refreshAssociations?(workspaceId: string, signal: AbortSignal): Promise<void>;
    unlink?(context: {
      workspaceId: string;
      taskId: string;
      reviewKey: string;
      connectionScope: string;
      repositoryId: string;
      changeRequestNumber: string | number;
      signal: AbortSignal;
    }): Promise<void>;
    ReviewPanel: Component<{
      panelId: string;
      presentation: "desktop" | "mobile";
      workspaceId: string;
      taskId: string;
      sessionId?: string;
      reviewKey: string;
      connectionScope: string;
      repositoryId: string;
      changeRequestNumber: string | number;
    }>;
    Selector?: Component;
    EmptyState?: Component;
  }): void;
  registerTaskPanel(registration: TaskPanelRegistration): void;
  registerTaskMenuAction(registration: TaskMenuActionRegistration): void;
  registerTaskFilter(registration: TaskFilterRegistration): void;
  registerTaskListFacet(registration: TaskListFacetRegistration): void;
}

export type PluginHost = PluginHostApi;

export interface KandevPlugin {
  initialize(registry: PluginRegistry, host: PluginHostApi): void | Promise<void>;
  destroy?(): void;
}
