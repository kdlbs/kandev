import { redirect } from "next/navigation";
import { getOnboardingState } from "@/lib/api/domains/orchestrate-api";
import { listAgents } from "@/lib/api/domains/settings-api";
import { SetupWizard } from "./setup-wizard";
import type { Agent } from "@/lib/types/http";

export default async function SetupPage() {
  const onboarding = await getOnboardingState({ cache: "no-store" }).catch(() => ({
    completed: false,
  }));
  if (onboarding.completed) {
    redirect("/orchestrate");
  }

  const agentsResponse = await listAgents({ cache: "no-store" }).catch(() => ({
    agents: [] as Agent[],
    total: 0,
  }));

  const profiles = agentsResponse.agents.flatMap((agent) =>
    agent.profiles.map((p) => ({
      id: p.id,
      label: `${agent.name} - ${p.name}`,
      agentName: agent.name,
    })),
  );

  return <SetupWizard agentProfiles={profiles} />;
}
