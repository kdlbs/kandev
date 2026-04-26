import { listIssues } from "@/lib/api/domains/orchestrate-api";
import { getActiveWorkspaceId } from "../lib/get-active-workspace";
import { IssuesPageClient } from "./issues-page-client";
import type { OrchestrateIssue } from "@/lib/state/slices/orchestrate/types";

export default async function IssuesPage() {
  const workspaceId = await getActiveWorkspaceId();

  let issues: OrchestrateIssue[] = [];
  if (workspaceId) {
    const res = await listIssues(workspaceId, { cache: "no-store" }).catch(() => ({
      issues: [],
    }));
    issues = res.issues ?? [];
  }

  return <IssuesPageClient initialIssues={issues} />;
}
