import { randomUUID } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { expect } from "../../fixtures/test-base";

export const OBSERVER_PLUGIN_ID = "kandev-plugin-observer";
const OBSERVER_ROOTS = [
  path.resolve(__dirname, "../../../../../kandev-plugin-observer"),
  path.resolve(__dirname, "../../../../../../kandev-plugin-observer"),
];
const OBSERVER_ROOT =
  OBSERVER_ROOTS.find((root) => existsSync(path.join(root, "manifest.yaml"))) ?? OBSERVER_ROOTS[0];
const OBSERVER_MANIFEST = path.join(OBSERVER_ROOT, "manifest.yaml");

function packagePath(): string {
  if (!existsSync(OBSERVER_MANIFEST))
    throw new Error(`Observer checkout is missing: ${OBSERVER_ROOT}`);
  const manifest = readFileSync(OBSERVER_MANIFEST, "utf8");
  const version = manifest.match(/^version:\s*["']?([^"'\s]+)["']?\s*$/m)?.[1];
  if (!version) throw new Error(`Observer version is missing from ${OBSERVER_MANIFEST}`);
  const result = path.join(OBSERVER_ROOT, `${OBSERVER_PLUGIN_ID}-${version}.tar.gz`);
  if (!existsSync(result))
    throw new Error(
      `Observer package is missing: ${result}. Run make verify-package-host in ${OBSERVER_ROOT}.`,
    );
  return result;
}

export async function installAndGrantObserver(
  page: Page,
  apiClient: ApiClient,
  workspaceId: string,
): Promise<void> {
  await page.goto("/settings/plugins");
  await page.getByTestId("install-plugin-trigger").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeVisible();
  await page.getByTestId("install-plugin-tab-upload").click();
  await page.getByTestId("install-plugin-file-input").setInputFiles(packagePath());
  await page.getByTestId("install-plugin-upload-submit").click();
  await expect(page.getByTestId("install-plugin-dialog")).toBeHidden({ timeout: 30_000 });
  await expect(page.getByTestId(`plugin-row-${OBSERVER_PLUGIN_ID}`)).toBeVisible({
    timeout: 30_000,
  });

  const context = await readApprovalContext(apiClient, workspaceId);
  const expected = ["host.v2.read:tasks", "host.v2.write:tasks"];
  expect(context.declared_capability_ids.sort()).toEqual(expected.sort());
  const response = await apiClient.rawRequest(
    "PUT",
    `/api/plugins/${OBSERVER_PLUGIN_ID}/capability-approvals`,
    {
      workspace_id: workspaceId,
      expected_revision: context.approval?.revision ?? 0,
      manifest_digest: context.manifest_digest,
      capability_ids: context.declared_capability_ids,
      reason: "Allow the observer to read tasks and create only approved proposals",
      audit_id: `observer-e2e-${randomUUID()}`,
    },
  );
  if (!response.ok)
    throw new Error(`Could not grant observer task capabilities: ${response.status}`);
}

export async function readApprovalContext(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<{
  manifest_digest: string;
  declared_capability_ids: string[];
  approval: { revision: number; capability_ids: string[]; state: string } | null;
}> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/plugins/${OBSERVER_PLUGIN_ID}/capability-approvals?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  if (!response.ok)
    throw new Error(`Could not read observer capability context: ${response.status}`);
  return (await response.json()) as {
    manifest_digest: string;
    declared_capability_ids: string[];
    approval: { revision: number; capability_ids: string[]; state: string } | null;
  };
}

export async function invokeObserverAction<T>(
  apiClient: ApiClient,
  workspaceId: string,
  action: string,
  body: Record<string, unknown> = {},
): Promise<T> {
  const response = await apiClient.rawRequest(
    "POST",
    `/api/plugins/${OBSERVER_PLUGIN_ID}/actions/${action}`,
    {
      workspaceId,
      body,
    },
  );
  const text = await response.text();
  if (!response.ok) throw new Error(`Observer ${action} failed (${response.status}): ${text}`);
  return JSON.parse(text) as T;
}

export async function uninstallObserver(apiClient: ApiClient): Promise<void> {
  await apiClient.rawRequest("DELETE", `/api/plugins/${OBSERVER_PLUGIN_ID}`).catch(() => undefined);
}
