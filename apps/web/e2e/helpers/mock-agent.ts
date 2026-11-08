import type { Agent } from "../../lib/types/http-agents";

export function getMockAgentId(agents: readonly Pick<Agent, "id" | "name">[]): string {
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("No mock-agent available in E2E test fixtures");
  return mockAgent.id;
}
