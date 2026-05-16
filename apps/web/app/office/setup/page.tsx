import { redirect } from "next/navigation";
import { getOnboardingState } from "@/lib/api/domains/office-api";
import { fetchUserSettings, listAgents } from "@/lib/api/domains/settings-api";
import { listWorkspaces } from "@/lib/api/domains/workspace-api";
import { SetupWizard } from "./setup-wizard";
import type { Agent } from "@/lib/types/http";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";

export default async function SetupPage({
  searchParams,
}: {
  searchParams: Promise<{ mode?: string }> | { mode?: string };
}) {
  const params = await searchParams;
  const isNewMode = params.mode === "new";

  const onboarding = await getOnboardingState({ cache: "no-store" }).catch(() => ({
    completed: false,
    fsWorkspaces: [],
  }));
  if (onboarding.completed && !isNewMode) {
    redirect("/office");
  }

  const [agentsResponse, userSettings, workspacesResponse] = await Promise.all([
    listAgents({ cache: "no-store" }).catch(() => ({
      agents: [] as Agent[],
      total: 0,
    })),
    fetchUserSettings({ cache: "no-store" }).catch(() => null),
    listWorkspaces({ cache: "no-store" }).catch(() => ({ workspaces: [] })),
  ]);

  const profiles = agentsResponse.agents.flatMap((agent) =>
    agent.profiles.map((p) => toAgentProfileOption(agent, p)),
  );

  // Pick a sensible default: the user's configured "utility" agent if any,
  // otherwise the first installed CLI's first profile. This way the office
  // wizard doesn't force the user to think about CLI selection from scratch.
  const defaultProfileId = pickDefaultProfile(
    userSettings?.settings?.default_utility_agent_id,
    profiles,
  );

  const fsWorkspaces = onboarding.fsWorkspaces ?? [];

  // Office workspaces have an office_workflow_id; kanban-only workspaces don't.
  // Only the office set should influence the suggested name.
  const existingOfficeNames = workspacesResponse.workspaces
    .filter((ws) => ws.office_workflow_id)
    .map((ws) => ws.name);
  const suggestedWorkspaceName = suggestWorkspaceName(existingOfficeNames);

  return (
    <SetupWizard
      agentProfiles={profiles}
      fsWorkspaces={fsWorkspaces}
      mode={params.mode}
      defaultAgentProfileId={defaultProfileId}
      suggestedWorkspaceName={suggestedWorkspaceName}
    />
  );
}

function suggestWorkspaceName(existing: string[]): string {
  const base = "Default";
  const taken = new Set(existing);
  if (!taken.has(base)) return base;
  for (let i = 2; i < 1000; i++) {
    const candidate = `${base} ${i}`;
    if (!taken.has(candidate)) return candidate;
  }
  return base;
}

function pickDefaultProfile(
  preferred: string | undefined | null,
  profiles: Array<{ id: string; agent_id: string }>,
): string {
  if (!profiles.length) return "";
  if (preferred) {
    // The user-settings field stores an agent id (CLI client). Pick the first
    // profile under that agent if it exists.
    const match = profiles.find((p) => p.agent_id === preferred);
    if (match) return match.id;
  }
  return profiles[0]?.id ?? "";
}
