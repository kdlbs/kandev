import type { TaskSwitcherItem } from "./task-switcher";

// Set to true to render mock data covering all sidebar edge cases (prototype mode)
export const MOCK_SIDEBAR = false;

const MOCK_REPO = "kdlbs/kandev";
const n = Date.now();
const mins = (m: number) => new Date(n - m * 60 * 1000).toISOString();
const hrs = (h: number) => new Date(n - h * 60 * 60 * 1000).toISOString();
const base = { primarySessionId: null as null, isArchived: false } as const;

/* prettier-ignore */
export const MOCK_ITEMS: TaskSwitcherItem[] = [
  { ...base, id: "mock-1", title: "Full stack authentication migration", state: "IN_PROGRESS", sessionState: "RUNNING", repositories: [MOCK_REPO, "kdlbs/kandev-web", "kdlbs/infra"], diffStats: { additions: 88, deletions: 12 }, updatedAt: mins(2), createdAt: hrs(3) },
  { ...base, id: "mock-1a", title: "Migrate auth endpoints to new provider", state: "IN_PROGRESS", sessionState: "RUNNING", diffStats: { additions: 24, deletions: 8 }, parentTaskId: "mock-1", updatedAt: mins(1), createdAt: hrs(2) },
  { ...base, id: "mock-1b", title: "Update frontend auth flows", parentTaskId: "mock-1", createdAt: hrs(1) },
  { ...base, id: "mock-2", title: "Fix task sidebar layout", state: "IN_PROGRESS", sessionState: "WAITING_FOR_INPUT", repositoryPath: MOCK_REPO, diffStats: { additions: 3, deletions: 1 }, updatedAt: mins(5), createdAt: hrs(4), prInfo: { number: 547, state: "Open" } },
  { ...base, id: "mock-2a", title: "Extract RepoGroupHeader component", sessionState: "WAITING_FOR_INPUT", repositoryPath: MOCK_REPO, parentTaskId: "mock-2", diffStats: { additions: 45, deletions: 3 }, updatedAt: mins(10), createdAt: hrs(3) },
  { ...base, id: "mock-3", title: "Refactor token usage in CLI", repositoryPath: MOCK_REPO, createdAt: hrs(5) },
  { ...base, id: "mock-4", title: "Update dependencies", state: "COMPLETED", sessionState: "COMPLETED", repositoryPath: MOCK_REPO, diffStats: { additions: 466, deletions: 124 }, updatedAt: hrs(2), createdAt: hrs(6), prInfo: { number: 138, state: "Merged" } },
  { ...base, id: "mock-5", title: "Implement feature X with full test coverage", state: "IN_PROGRESS", sessionState: "RUNNING", repositoryPath: "myorg/other-repo", diffStats: { additions: 11, deletions: 3 }, updatedAt: mins(0.5), createdAt: hrs(1) },
  { ...base, id: "mock-5a", title: "Add unit tests", repositoryPath: "myorg/other-repo", parentTaskId: "mock-5", createdAt: mins(30) },
  { ...base, id: "mock-6", title: "Draft task — no repo assigned yet", createdAt: hrs(7) },
];
