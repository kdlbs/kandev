"use client";

import { use } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfile } from "@/lib/state/slices/office/selectors";
import { AgentInstructionsTab } from "../components/agent-instructions-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentInstructionsPage({ params }: Props) {
  const { id } = use(params);
  const agent = useAppStore((s) => selectOfficeAgentProfile(s, id));
  if (!agent) return null;
  return <AgentInstructionsTab agent={agent} />;
}
