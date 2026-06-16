import { getOnboardingState } from "@/lib/api/domains/office-api";
import type { OnboardingFSWorkspace } from "@/lib/api/domains/office-api";
import { fetchUserSettings, listAgents } from "@/lib/api/domains/settings-api";
import { listWorkspaces } from "@/lib/api/domains/workspace-api";
import type { AgentProfileOption } from "@/lib/state/slices/settings/types";
import { toAgentProfileOption } from "@/lib/state/slices/settings/types";
import type { Agent } from "@/lib/types/http";

export type SetupWizardRouteProps = {
  agentProfiles: AgentProfileOption[];
  fsWorkspaces: OnboardingFSWorkspace[];
  mode?: string;
  defaultAgentProfileId: string;
  suggestedWorkspaceName: string;
};

export type SetupRouteData =
  | { kind: "redirect"; href: "/office" }
  | { kind: "wizard"; props: SetupWizardRouteProps };

export async function loadSetupRouteData(mode?: string): Promise<SetupRouteData> {
  const isNewMode = mode === "new";
  const requestOptions = { cache: "no-store" as const };

  const onboarding = await getOnboardingState(requestOptions).catch(() => ({
    completed: false,
    fsWorkspaces: [],
  }));
  if (onboarding.completed && !isNewMode) {
    return { kind: "redirect", href: "/office" };
  }

  const [agentsResponse, userSettings, workspacesResponse] = await Promise.all([
    listAgents(requestOptions).catch(() => ({
      agents: [] as Agent[],
      total: 0,
    })),
    fetchUserSettings(requestOptions).catch(() => null),
    listWorkspaces(requestOptions).catch(() => ({ workspaces: [] })),
  ]);

  const profiles = agentsResponse.agents.flatMap((agent) =>
    agent.profiles.map((p) => toAgentProfileOption(agent, p)),
  );

  const defaultProfileId = pickDefaultProfile(
    userSettings?.settings?.default_utility_agent_id,
    profiles,
  );
  const fsWorkspaces = onboarding.fsWorkspaces ?? [];
  const existingOfficeNames = workspacesResponse.workspaces
    .filter((ws) => ws.office_workflow_id)
    .map((ws) => ws.name);

  return {
    kind: "wizard",
    props: {
      agentProfiles: profiles,
      fsWorkspaces,
      mode,
      defaultAgentProfileId: defaultProfileId,
      suggestedWorkspaceName: suggestWorkspaceName(existingOfficeNames),
    },
  };
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
    const match = profiles.find((p) => p.agent_id === preferred);
    if (match) return match.id;
  }
  return profiles[0]?.id ?? "";
}
