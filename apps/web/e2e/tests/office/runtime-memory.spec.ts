import { test, expect } from "../../fixtures/office-fixture";

test.describe("Office runtime memory", () => {
  test("allows scoped memory reads and writes and records the write on the run", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const seededRun = await apiClient.seedRun({
      agentProfileId: officeSeed.agentId,
      status: "claimed",
      reason: "heartbeat",
      sessionId: "runtime-memory-session",
    });
    const { token } = await apiClient.mintRuntimeToken({
      agentProfileId: officeSeed.agentId,
      workspaceId: officeSeed.workspaceId,
      runId: seededRun.run_id,
      sessionId: "runtime-memory-session",
      capabilities: JSON.stringify({ read_memory: true, write_memory: true }),
    });
    const memoryPath = `/workspaces/${officeSeed.workspaceId}/memory/agents/${officeSeed.agentId}/knowledge/runtime-note`;

    const put = await apiClient.runtimePutMemory(token, memoryPath, "memory from runtime");
    expect(put.status).toBe(200);
    const get = await apiClient.runtimeGetMemory(token, memoryPath);
    expect(get.status).toBe(200);
    const body = (await get.json()) as { memory: { content: string } };
    expect(body.memory.content).toBe("memory from runtime");

    await testPage.goto(`/office/agents/${officeSeed.agentId}/runs/${seededRun.run_id}`);
    await expect(testPage.getByTestId("events-log")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByText("runtime.action")).toBeVisible();
    await expect(testPage.getByText(/write_memory/)).toBeVisible();
  });
});
