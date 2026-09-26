"use client";

import { use } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfile } from "@/lib/state/slices/office/selectors";
import { AgentSkillsTab } from "../components/agent-skills-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentSkillsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useAppStore((s) => selectOfficeAgentProfile(s, id));
  if (!agent) return null;
  return <AgentSkillsTab agent={agent} />;
}
