import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";
import sonarjs from "eslint-plugin-sonarjs";
import unusedImports from "eslint-plugin-unused-imports";

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Test artifacts (Playwright):
    "**/test-results/**",
    "**/playwright-report/**",
  ]),
  {
    plugins: {
      sonarjs,
      "unused-imports": unusedImports,
    },
    rules: {
      "max-lines": ["warn", { max: 600, skipBlankLines: true, skipComments: true }],
      "max-lines-per-function": ["warn", { max: 100, skipBlankLines: true, skipComments: true }],
      complexity: ["warn", 15],
      "max-depth": ["warn", 4],
      "max-params": ["warn", 5],
      "no-nested-ternary": "warn",
      "sonarjs/cognitive-complexity": ["warn", 20],
      "sonarjs/no-duplicate-string": ["warn", { threshold: 4 }],
      "sonarjs/no-identical-functions": "warn",
      "unused-imports/no-unused-imports": "warn",
      "@typescript-eslint/no-unused-vars": [
        "warn",
        { varsIgnorePattern: "^_", argsIgnorePattern: "^_", caughtErrorsIgnorePattern: "^_" },
      ],
      // Guard against re-introducing Zustand mirror reads for server state that
      // has migrated to TanStack Query. Server GitHub state (taskPRs, watches,
      // presets, status, rate limit) lives in the TQ cache — read it via the
      // hooks in hooks/domains/github/. Only client-only github state
      // (pendingPrUrlByTaskId) remains in the Zustand slice.
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "MemberExpression[object.name=/^(state|s)$/][property.name=/^(taskPRs|githubStatus|prWatches|reviewWatches|issueWatches|actionPresets|prFeedbackCache)$/]",
          message:
            "GitHub server state has migrated to TanStack Query. Read it via hooks/domains/github/ (e.g. useTaskPRs, useGitHubStatus, useReviewWatches), not the Zustand mirror.",
        },
        {
          // The office Zustand mirror is fully removed; office server state
          // lives in the TanStack Query cache. Ban reads of `state.office` /
          // `s.office` and the old office slice action selectors.
          selector:
            "MemberExpression[object.name=/^(state|s)$/][property.name=/^(office|setOfficeAgentProfiles|addOfficeAgentProfile|updateOfficeAgentProfile|removeOfficeAgentProfile|setSkills|addSkill|removeSkill|setProjects|addProject|updateProject|removeProject|setApprovals|setActivity|setCostSummary|setBudgetPolicies|setRoutines|setInboxItems|setInboxCount|setRuns|setDashboard|setTasks|appendTasks|patchTaskInStore|setTaskFilters|setTaskViewMode|setTaskSortField|setTaskSortDir|setTaskGroupBy|toggleNesting|setTasksLoading|setOfficeLoading|setOfficeRefetchTrigger|setWorkspaceRouting|setKnownProviders|setRoutingPreview|setProviderHealth|upsertProviderHealth|setRunAttempts|appendRunAttempt|setAgentRouting)$/]",
          message:
            "Office server state has migrated to TanStack Query. Read it via hooks/domains/office/ (e.g. useOfficeAgents, useOfficeProjects, useOfficeInboxItems) or officeQueryOptions, not the Zustand mirror.",
        },
        {
          // Workspace server state migrated to TanStack Query. The repositories /
          // repositoryBranches / repositoryScripts slices were removed; only the
          // client-only `workspaces.activeId` selection remains in Zustand.
          selector:
            "MemberExpression[object.name=/^(state|s)$/][property.name=/^(repositories|repositoryBranches|repositoryScripts|setWorkspaces|setRepositories|setRepositoriesLoading|setRepositoryBranches|setRepositoryBranchesLoading|setRepositoryBranchesFetchError|setRepositoryScripts|setRepositoryScriptsLoading|clearRepositoryScripts|invalidateRepositories)$/]",
          message:
            "Workspace server state has migrated to TanStack Query. Read it via hooks/domains/workspace/ (useWorkspaces, useRepositories, useAllRepositories, useBranches, useRepositoryScripts), not the Zustand mirror.",
        },
        {
          // Ban reads of the migrated workspaces list (`state.workspaces.items`).
          // `state.workspaces.activeId` stays — it is client-only selection state.
          selector: "MemberExpression[object.property.name='workspaces'][property.name='items']",
          message:
            "The workspaces list moved to TanStack Query. Read it via useWorkspaces() from hooks/domains/workspace/. Only workspaces.activeId stays in Zustand.",
        },
        {
          // Kanban server state migrated to TanStack Query: the single-workflow
          // snapshot (`state.kanban`), all-workflow snapshots (`state.kanbanMulti`)
          // and the workflows list (`workflows.items`) live in the TQ cache. Only
          // the client-only active-workflow selection (`workflows.activeId`) and
          // active task/session selection (`state.tasks.*`) remain in Zustand.
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(kanban|kanbanMulti|setWorkflows|reorderWorkflowItems|setWorkflowSnapshot|setKanbanMultiSnapshots|setKanbanMultiLoading|clearKanbanMulti|updateMultiTask|removeMultiTask)$/]",
          message:
            "Kanban server state has migrated to TanStack Query. Read snapshots via hooks/domains/kanban/use-kanban-snapshots (useKanbanMultiSnapshots, useWorkflowItems, useActiveTaskWorkflow, useKanbanSnapshotMutator) or the kanban query-options, not the Zustand mirror. Only workflows.activeId / tasks.* stay in Zustand.",
        },
        {
          // Ban reads of the migrated workflows list (`workflows.items`).
          // `workflows.activeId` stays — it is client-only selection state.
          selector: "MemberExpression[object.property.name='workflows'][property.name='items']",
          message:
            "The workflows list moved to TanStack Query. Read it via useWorkflowItems() / useWorkflows() from hooks/domains/kanban/. Only workflows.activeId stays in Zustand.",
        },
        {
          // Settings server state migrated to TanStack Query; the Zustand
          // `settings` slice is fully removed. Ban reads of the distinctive
          // settings fields and ALL its old action selectors. (Common field
          // names like `userSettings`/`editors`/`prompts`/`secrets`/`sprites`
          // are intentionally omitted from the read list to avoid false
          // positives on unrelated local objects — the slice deletion + tsc are
          // the primary guarantee for those; the banned setters below cover the
          // mirror-write path.)
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(executors|settingsAgents|agentDiscovery|availableAgents|agentProfiles|installJobs|notificationProviders|settingsData|setExecutors|setSettingsAgents|setAgentDiscovery|setAgentDiscoveryLoading|setAvailableAgents|setAvailableAgentsLoading|setAgentProfiles|setInstallJobs|upsertInstallJob|appendInstallOutput|clearInstallJob|setEditors|setEditorsLoading|setPrompts|setPromptsLoading|setSecrets|setSecretsLoading|addSecret|updateSecret|removeSecret|setSpritesStatus|setSpritesInstances|setSpritesLoading|removeSpritesInstance|setNotificationProviders|setNotificationProvidersLoading|setSettingsData|setUserSettings|bumpAgentProfilesVersion)$/]",
          message:
            "Settings server state has migrated to TanStack Query. Read it via hooks/domains/settings/ (useExecutors, useAgentProfiles, useSettingsAgents, useUserSettings, useEditors, useCustomPrompts, useSecrets, useSprites, useNotificationProviders, useAvailableAgents, useAgentDiscovery) or settingsQueryOptions, not the Zustand mirror.",
        },
        {
          // Session (D4) server state lives in the TanStack Query cache: the
          // by-id (`state.taskSessions`) and by-task (`state.taskSessionsByTask`)
          // session maps. Read them via hooks/domains/session/use-task-session-by-id
          // (useTaskSessionById, useTaskSessionsByTask), not the Zustand mirror.
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(taskSessions|taskSessionsByTask)$/]",
          message:
            "Session (taskSessions) server state has migrated to TanStack Query. Read it via hooks/domains/session/use-task-session-by-id (useTaskSessionById, useTaskSessionsByTask), not the Zustand mirror.",
        },
        {
          // Session-runtime (D6) server fields migrated to TanStack Query: git
          // status, session mode, session models, session todos, and poll mode
          // (qk.session.*, see query-options/session-runtime.ts). Their Zustand
          // slice fields + setters were removed. Client-only runtime state
          // (shell/process/terminal streams, environmentIdBySessionId index,
          // activeModel selection) stays in Zustand and is intentionally omitted.
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(gitStatus|sessionMode|sessionModels|sessionTodos|sessionPollMode|prepareProgress|setGitStatus|clearGitStatus|setSessionMode|clearSessionMode|setSessionModels|setSessionTodos|setSessionPollMode)$/]",
          message:
            "Session-runtime server state (gitStatus, sessionMode, sessionModels, sessionTodos, sessionPollMode, prepareProgress) has migrated to TanStack Query. Read it via the session-runtime query-options / hooks (useSessionGitStatus, useSessionMode, useSessionModels, useSessionTodos, prepareProgressQueryOptions, sessionRuntimeQueryOptions), not the Zustand mirror.",
        },
        {
          // The agentctl status badge (starting / ready / error) migrated to
          // TanStack Query (qk.session.agentctl). The Zustand `sessionAgentctl`
          // slice + its setter were removed. Read it via the session-runtime
          // query-options / hook (sessionAgentctlQueryOptions, useSessionAgentctl)
          // and write it via writeAgentctlStatus from lib/query/agentctl-status.
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(sessionAgentctl|setSessionAgentctlStatus)$/]",
          message:
            "agentctl status has migrated to TanStack Query (qk.session.agentctl). Read it via useSessionAgentctl / sessionAgentctlQueryOptions and write it via writeAgentctlStatus (lib/query/agentctl-status), not the Zustand mirror.",
        },
        {
          // Task-plan server state migrated to TanStack Query: the plan itself
          // (qk.taskSession.plans) and the revisions list
          // (qk.taskSession.plansRevisions). The Zustand `taskPlans` server
          // sub-fields + their setters were removed. Only the CLIENT-only
          // sub-fields stay (savingByTaskId, revisionContentCache,
          // previewRevisionIdByTaskId, comparePairByTaskId,
          // lastSeenUpdatedAtByTaskId), so the slice object itself is NOT banned.
          selector:
            "MemberExpression[object.property.name='taskPlans'][property.name=/^(byTaskId|loadingByTaskId|loadedByTaskId|revisionsByTaskId|revisionsLoadingByTaskId|revisionsLoadedByTaskId)$/]",
          message:
            "Task-plan server state has migrated to TanStack Query. Read the plan via useTaskPlan / taskPlanQueryOptions and revisions via taskPlanRevisionsQueryOptions (qk.taskSession.plans / plansRevisions), not the Zustand mirror. Only the client-only taskPlans.{savingByTaskId,revisionContentCache,previewRevisionIdByTaskId,comparePairByTaskId,lastSeenUpdatedAtByTaskId} stay in Zustand.",
        },
        {
          // Ban the removed task-plan server-state setters.
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(setTaskPlan|setTaskPlanLoading|setPlanRevisions|upsertPlanRevision|setPlanRevisionsLoading)$/]",
          message:
            "These task-plan server-state setters were removed (migrated to TanStack Query). Write the plan/revisions caches via the bridge or queryClient.setQueryData (qk.taskSession.plans / plansRevisions), not the Zustand mirror.",
        },
        {
          // Worktree server data is derived from the canonical TaskSession TQ
          // cache (worktree_id / worktree_path / worktree_branch). The Zustand
          // `worktrees` / `sessionWorktreesBySessionId` slices + their setters
          // were removed. Read worktrees via hooks/domains/session/use-session-worktrees
          // (useSessionWorktrees) or hooks/use-worktree (useWorktree).
          selector:
            "MemberExpression[object.name=/^(state|s|draft)$/][property.name=/^(worktrees|sessionWorktreesBySessionId|setWorktree|setSessionWorktrees)$/]",
          message:
            "Worktree state is derived from the TaskSession TQ cache (worktree_* fields). Read it via useSessionWorktrees (hooks/domains/session/use-session-worktrees) or useWorktree (hooks/use-worktree), not the removed Zustand worktrees / sessionWorktreesBySessionId mirror.",
        },
      ],
    },
  },
  // The github slice itself + its tests may still reference the client-only
  // pendingPrUrlByTaskId field; the guard only targets the migrated server fields.
  // The state-hydrator + SSR builders legitimately read the SSR workspace
  // snapshot (workspaces.items / repositories) to seed the TanStack Query cache.
  {
    files: [
      "lib/state/slices/github/**/*.ts",
      "components/state-hydrator.tsx",
      "lib/ssr/**/*.ts",
      // The hydrator reads SSR-provided settings fields (e.g.
      // userSettings.sidebarViews) off the SsrInitialState `state` param to seed
      // client-only UI slices; not a Zustand mirror read.
      "lib/state/hydration/**/*.ts",
      "**/*.test.ts",
      "**/*.test.tsx",
    ],
    rules: {
      "no-restricted-syntax": "off",
    },
  },
  // E2E tests (Playwright): disable React hooks rules since Playwright's `use()` and
  // `test.extend()` patterns are falsely flagged, and relax test-specific limits.
  {
    files: ["e2e/**/*.ts"],
    rules: {
      "react-hooks/rules-of-hooks": "off",
      "react-hooks/exhaustive-deps": "off",
      "max-lines-per-function": "off",
      "max-lines": "off",
      "sonarjs/no-duplicate-string": "off",
    },
  },
]);

export default eslintConfig;
