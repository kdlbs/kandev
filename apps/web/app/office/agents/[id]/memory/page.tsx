"use client";

import { use } from "react";
import { useAppStore } from "@/components/state-provider";
import { selectOfficeAgentProfile } from "@/lib/state/slices/office/selectors";
import { AgentMemoryTab } from "../components/agent-memory-tab";

type Props = { params: Promise<{ id: string }> };

export default function AgentMemoryPage({ params }: Props) {
  const { id } = use(params);
  const agent = useAppStore((s) => selectOfficeAgentProfile(s, id));
  if (!agent) return null;
  return <AgentMemoryTab agent={agent} />;
}
