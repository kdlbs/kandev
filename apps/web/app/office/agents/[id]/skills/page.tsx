"use client";

import { use } from "react";
import { AgentSkillsTab } from "../components/agent-skills-tab";
import { useOfficeAgentProfile } from "../use-agent-detail-data";

type Props = { params: Promise<{ id: string }> };

export default function AgentSkillsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useOfficeAgentProfile(id);
  if (!agent) return null;
  return <AgentSkillsTab agent={agent} />;
}
