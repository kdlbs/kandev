import { expect, type APIRequestContext } from "@playwright/test";

/** The worker's task reset does not remove workspace orchestrator registrations. */
export async function resetWorkspaceOrchestrators(
  request: APIRequestContext,
  baseUrl: string,
  workspaceId: string,
): Promise<void> {
  const url = `${baseUrl}/api/v1/orchestration/workspaces/${workspaceId}/orchestrators`;
  const response = await request.get(url);
  expect(response.ok()).toBeTruthy();
  const { orchestrators } = (await response.json()) as { orchestrators: { id: string }[] };
  for (const orchestrator of orchestrators) {
    const deleted = await request.delete(`${url}/${orchestrator.id}`);
    expect(deleted.ok()).toBeTruthy();
  }
}
