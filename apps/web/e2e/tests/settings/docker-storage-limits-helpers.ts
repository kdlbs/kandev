import type { Page } from "@playwright/test";
import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export type DockerExecutorType = "local_docker" | "remote_docker";

export async function createDockerStorageProfile(
  apiClient: ApiClient,
  type: DockerExecutorType,
  suffix: string,
  config: Record<string, string> = {},
) {
  const executor = await apiClient.createExecutor(
    `e2e-storage-${type}-${suffix}`,
    type,
    type === "remote_docker"
      ? {
          ssh_host: "build-box.invalid",
          ssh_user: "kandev",
          ssh_identity_source: "agent",
          ssh_host_fingerprint: "SHA256:e2e-pinned",
        }
      : undefined,
  );
  const profile = await apiClient.createExecutorProfile(executor.id, {
    name: `storage-${suffix}`,
    config: {
      custom_storage_test_key: "keep-me",
      dockerfile: "FROM busybox\nWORKDIR /workspace\n",
      image_tag: "kandev/storage:e2e",
      ...config,
    },
  });
  return { executor, profile };
}

export async function deleteDockerStorageProfile(
  apiClient: ApiClient,
  executorId: string,
  profileId: string,
) {
  await apiClient.deleteExecutorProfile(profileId).catch(() => {});
  await apiClient.deleteExecutor(executorId).catch(() => {});
}

export async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
}

export async function expectTouchTarget(locator: ReturnType<Page["getByRole"]>) {
  await locator.scrollIntoViewIfNeeded();
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  if (box) {
    expect(box.height).toBeGreaterThanOrEqual(44);
    expect(box.width).toBeGreaterThanOrEqual(44);
  }
}
