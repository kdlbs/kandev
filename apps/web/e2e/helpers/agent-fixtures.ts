type AgentFixture = {
  id: string;
  name: string;
};

export function getMockAgent<T extends AgentFixture>(agents: T[]): T {
  const mockAgent = agents.find((agent) => agent.name === "mock-agent");
  if (!mockAgent) throw new Error("mock-agent unavailable in test fixtures");
  return mockAgent;
}
