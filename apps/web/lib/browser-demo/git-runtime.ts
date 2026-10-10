import {
  type CumulativeDiff,
  type FileInfo,
  type SessionCommit,
} from "@/lib/state/slices/session-runtime/types";
import { fileReviewsBySession, notify, state } from "./runtime-state";
import { DEMO_IDS } from "./scenario";

export function demoGitData(sessionId: string): DemoGitData {
  const task = state.tasks.find((candidate) => candidate.primary_session_id === sessionId);
  if (task?.id === "demo-task-checkout") return checkoutGitData(sessionId);
  if (task?.id === "demo-task-audit") return auditGitData(sessionId);
  if (
    task?.state === "REVIEW" &&
    state.messagesBySession[sessionId]?.some((message) => message.id === `${sessionId}-summary`)
  ) {
    return completedDemoTaskGitData(sessionId, task.id);
  }
  return emptyGitData(sessionId, task?.id ?? "task");
}

export function checkoutGitData(sessionId: string): DemoGitData {
  const files: Record<string, FileInfo> = {
    "src/checkout/complete-order.ts": {
      path: "src/checkout/complete-order.ts",
      status: "modified",
      staged: false,
      additions: 8,
      deletions: 4,
      diff: "@@ -42,7 +42,11 @@ export async function completeOrder(order: Order) {\n-  await withOrderLock(order.id, completeOrder);\n+  await completePayment(order);\n+  await withOrderLock(order.id, async () => {\n+    await reserveInventory(order);\n+  });",
    },
    "tests/checkout/concurrent-inventory.test.ts": {
      path: "tests/checkout/concurrent-inventory.test.ts",
      status: "added",
      staged: true,
      additions: 34,
      deletions: 0,
      diff: '@@ -0,0 +1,5 @@\n+describe("concurrent checkout", () => {\n+  it("does not hold the order lock during payment", async () => {\n+    await expect(runConcurrentCheckout()).resolves.toEqual(["paid", "reserved"]);\n+  });\n+});',
    },
  };
  return {
    status: makeGitStatus("kandev/fix-checkout-timeout", files, 1, 0),
    commits: [
      makeCommit(sessionId, {
        sha: "8f73b42",
        message: "test: cover concurrent checkout",
        filesChanged: 1,
        insertions: 34,
        deletions: 0,
      }),
    ],
    cumulativeDiff: makeCumulativeDiff(sessionId, files, 1),
  };
}

export function auditGitData(sessionId: string): DemoGitData {
  const files: Record<string, FileInfo> = {
    "src/audit/record-event.ts": {
      path: "src/audit/record-event.ts",
      status: "modified",
      staged: false,
      additions: 12,
      deletions: 5,
      diff: "@@ -44,7 +44,9 @@ export async function recordEvent(input: AuditInput) {\n-    actorIp: input.ip,\n+    actorRegion: await privacyFilter.regionFor(input.ip),\n+    retentionDays: 90,",
    },
    "src/pages/admin/activity-page.tsx": {
      path: "src/pages/admin/activity-page.tsx",
      status: "added",
      staged: true,
      additions: 68,
      deletions: 0,
      diff: "@@ -0,0 +1,6 @@\n+export function ActivityFeed({ events }: Props) {\n+  return events.map((event) => (\n+    <AuditEventRow key={event.id} event={event} />\n+  ));\n+}",
    },
  };
  return {
    status: makeGitStatus("kandev/audit-logging", files, 2, 0),
    commits: [
      makeCommit(sessionId, {
        sha: "c24e18a",
        message: "feat: persist privileged audit events",
        filesChanged: 4,
        insertions: 116,
        deletions: 19,
      }),
      makeCommit(sessionId, {
        sha: "41ac09d",
        message: "feat: add admin activity feed",
        filesChanged: 3,
        insertions: 68,
        deletions: 8,
      }),
    ],
    cumulativeDiff: makeCumulativeDiff(sessionId, files, 2),
  };
}

export function emptyGitData(sessionId: string, taskId: string): DemoGitData {
  return {
    status: makeGitStatus(`kandev/${taskId}`, {}, 0, 0),
    commits: [],
    cumulativeDiff: null,
  };
}

export function completedDemoTaskGitData(sessionId: string, taskId: string): DemoGitData {
  const files: Record<string, FileInfo> = {
    "src/pages/dashboard-page.tsx": {
      path: "src/pages/dashboard-page.tsx",
      status: "modified",
      staged: false,
      additions: 7,
      deletions: 1,
      diff: "@@ -8,1 +8,1 @@\n-return <main><h1>Operations</h1></main>;\n+return <main><h1>Operations</h1><ServiceHealthSummary /></main>;",
    },
    "tests/dashboard.test.tsx": {
      path: "tests/dashboard.test.tsx",
      status: "added",
      staged: false,
      additions: 28,
      deletions: 0,
      diff: '@@ -0,0 +1,5 @@\n+describe("service health", () => {\n+  it("shows current service details", () => {\n+    expect(renderDashboard()).toHaveTextContent("12 services healthy");\n+  });\n+});',
    },
  };
  return {
    status: makeGitStatus(`kandev/${taskId}`, files, 0, 0),
    commits: [],
    cumulativeDiff: makeCumulativeDiff(sessionId, files, 0),
  };
}

export function makeGitStatus(
  branch: string,
  files: Record<string, FileInfo>,
  ahead: number,
  behind: number,
) {
  const values = Object.values(files);
  return {
    branch,
    remote_branch: `origin/${branch}`,
    modified: values.filter((file) => file.status === "modified").map((file) => file.path),
    added: values.filter((file) => file.status === "added").map((file) => file.path),
    deleted: values.filter((file) => file.status === "deleted").map((file) => file.path),
    untracked: values.filter((file) => file.status === "untracked").map((file) => file.path),
    renamed: values.filter((file) => file.status === "renamed").map((file) => file.path),
    ahead,
    behind,
    files,
    branch_additions: values.reduce((total, file) => total + (file.additions ?? 0), 0),
    branch_deletions: values.reduce((total, file) => total + (file.deletions ?? 0), 0),
  };
}

export function makeCommit(
  sessionId: string,
  input: {
    sha: string;
    message: string;
    filesChanged: number;
    insertions: number;
    deletions: number;
  },
): SessionCommit {
  return {
    id: `demo-commit-${input.sha}`,
    session_id: sessionId,
    commit_sha: input.sha,
    parent_sha: "6c12a90",
    author_name: "Kandev Demo",
    author_email: "demo@kandev.com",
    commit_message: input.message,
    committed_at: "2026-07-18T11:45:00.000Z",
    files_changed: input.filesChanged,
    insertions: input.insertions,
    deletions: input.deletions,
    created_at: "2026-07-18T11:45:00.000Z",
    pushed: false,
  };
}

export function makeCumulativeDiff(
  sessionId: string,
  files: Record<string, FileInfo>,
  totalCommits: number,
): CumulativeDiff {
  return {
    session_id: sessionId,
    base_commit: "6c12a90",
    head_commit: "8f73b42",
    total_commits: totalCommits,
    files,
  };
}

export function notifyGitStatus(sessionId: string) {
  const session = state.sessions.find((candidate) => candidate.id === sessionId);
  if (!session) return;
  notify("session.git.event", {
    type: "status_update",
    session_id: sessionId,
    task_id: session.task_id,
    agent_id: DEMO_IDS.agent,
    timestamp: new Date().toISOString(),
    status: demoGitData(sessionId).status,
  });
}

export function serializeFileReviews(sessionId: string) {
  const now = "2026-07-18T12:00:00.000Z";
  return Array.from(fileReviewsBySession.get(sessionId) ?? [], ([filePath, review], index) => ({
    id: `demo-review-${index + 1}`,
    session_id: sessionId,
    file_path: filePath,
    reviewed: review.reviewed,
    diff_hash: review.diffHash,
    reviewed_at: review.reviewed ? now : null,
    created_at: now,
    updated_at: now,
  }));
}

export type DemoGitData = {
  status: {
    branch: string;
    remote_branch: string;
    modified: string[];
    added: string[];
    deleted: string[];
    untracked: string[];
    renamed: string[];
    ahead: number;
    behind: number;
    files: Record<string, FileInfo>;
    branch_additions: number;
    branch_deletions: number;
  };
  commits: SessionCommit[];
  cumulativeDiff: CumulativeDiff | null;
};
