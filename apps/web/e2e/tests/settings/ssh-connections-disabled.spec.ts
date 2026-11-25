import { randomUUID } from "node:crypto";
import { test, expect } from "../../fixtures/test-base";

test("disabled SSH management rejects direct API access and does not load credentials", async ({
  backend,
  testPage,
}) => {
  const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "false" });
  try {
    const response = await fetch(`${backend.baseUrl}/api/v1/ssh/managed/identities`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        request_id: randomUUID(),
        kind: "generate",
        label: "Disabled fixture",
      }),
    });
    expect(response.status).toBe(404);
    const credentialRequests: string[] = [];
    testPage.on("request", (request) => {
      if (/\/api\/v1\/ssh\/managed\/(identities|connections)(?:[/?]|$)/.test(request.url())) {
        credentialRequests.push(request.url());
      }
    });
    for (const path of [
      "/settings/ssh-connections",
      "/settings/executors/ssh-connections",
      "/settings/executors/new/ssh",
    ]) {
      await testPage.goto(path);
      await expect(
        testPage.getByText("SSH connection management is not available.", { exact: true }),
      ).toBeVisible();
    }
    await expect(testPage.getByLabel("Host", { exact: true })).toHaveCount(0);
    expect(credentialRequests).toEqual([]);
  } finally {
    await testPage.close();
    await release();
  }
});
