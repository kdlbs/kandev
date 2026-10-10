import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { expect, type Page } from "@playwright/test";

const WORKSPACE_CONTENTS = "recognized workspace fixture bytes";
const CHECKOUT_CONTENTS = "unclassified checkout fixture bytes";
const EXTERNAL_CONTENTS = "external symlink target fixture bytes";

export type WorkspaceStorageDiscoveryFixture = {
  workspaceRoot: string;
  workspaceFile: string;
  markerFile: string;
  markerContents: string;
  expectedBytes: number;
  checkoutRoot: string;
  checkoutFilePaths: string[];
  symlinkPath: string;
  externalGuide: string;
  cleanup: () => void;
};

export type WorkspaceStorageBaselineFixture = {
  expectedBytes: number;
  cleanup: () => void;
};

export function seedWorkspaceStorageBaseline(tmpDir: string): WorkspaceStorageBaselineFixture {
  const id = randomUUID();
  const workspaceName = `baseline-${id}_e2e`;
  const workspaceRoot = path.join(tmpDir, ".kandev", "tasks", workspaceName);
  const workspaceContents = "existing recognized workspace baseline bytes";
  const markerContents = JSON.stringify({
    task_id: `storage-baseline-${id}`,
    workspace_id: `storage-baseline-workspace-${id}`,
    task_dir_name: workspaceName,
    layout_version: 1,
    created_at: "2026-01-01T00:00:00Z",
  });
  fs.mkdirSync(workspaceRoot, { recursive: true });
  fs.writeFileSync(path.join(workspaceRoot, "source.txt"), workspaceContents);
  fs.writeFileSync(path.join(workspaceRoot, ".kandev-workspace.json"), markerContents);

  return {
    expectedBytes: Buffer.byteLength(workspaceContents) + Buffer.byteLength(markerContents),
    cleanup: () => fs.rmSync(workspaceRoot, { recursive: true, force: true }),
  };
}

export async function analyzeStorageAndWait(
  page: Page,
  activation: "click" | "tap",
): Promise<void> {
  const button = page.getByTestId("storage-analyze");
  const responsePromise = page.waitForResponse((response) => {
    const request = response.request();
    return (
      request.method() === "POST" &&
      new URL(response.url()).pathname === "/api/v1/system/storage/analyze"
    );
  });
  if (activation === "tap") await button.tap();
  else await button.click();

  const acceptedResponse = await responsePromise;
  if (!acceptedResponse.ok()) {
    throw new Error(`Storage analysis request failed: ${acceptedResponse.status()}`);
  }
  const accepted = (await acceptedResponse.json()) as { job_id: string };
  const jobUrl = new URL(
    `/api/v1/system/jobs/${encodeURIComponent(accepted.job_id)}`,
    page.url(),
  ).toString();
  await expect
    .poll(
      async () => {
        const response = await page.request.get(jobUrl);
        if (!response.ok()) return null;
        const job = (await response.json()) as { state?: string };
        return job.state ?? null;
      },
      { timeout: 60_000 },
    )
    .toBe("succeeded");
  await expect(button).toHaveAttribute("data-job-state", "succeeded");
}

export async function readWorkspaceStorageSummary(page: Page): Promise<{
  total_bytes: number;
  warnings: string[];
}> {
  return page.evaluate(async () => {
    const response = await fetch("/api/v1/system/storage");
    if (!response.ok) throw new Error(`Storage overview failed: ${response.status}`);
    const overview = await response.json();
    return overview.summary.workspaces;
  });
}

export function seedWorkspaceStorageDiscovery(tmpDir: string): WorkspaceStorageDiscoveryFixture {
  const id = randomUUID();
  const tasksRoot = path.join(tmpDir, ".kandev", "tasks");
  const workspaceName = `recognized-${id}`;
  const workspaceRoot = path.join(tasksRoot, workspaceName);
  const workspaceFile = path.join(workspaceRoot, "source.txt");
  const markerFile = path.join(workspaceRoot, ".kandev-workspace.json");
  const marker = JSON.stringify({
    task_id: `storage-discovery-${id}`,
    workspace_id: `storage-workspace-${id}`,
    task_dir_name: workspaceName,
    layout_version: 1,
    created_at: "2026-01-01T00:00:00Z",
  });
  fs.mkdirSync(workspaceRoot, { recursive: true });
  fs.writeFileSync(workspaceFile, WORKSPACE_CONTENTS);
  fs.writeFileSync(markerFile, marker);

  const checkoutRoot = path.join(tasksRoot, `unclassified-${id}`);
  const checkoutFilePaths = [".git", "apps", "node_modules"].map((directory) => {
    const root = path.join(checkoutRoot, directory);
    const fixtureFile = path.join(root, "keep.txt");
    fs.mkdirSync(root, { recursive: true });
    fs.writeFileSync(fixtureFile, CHECKOUT_CONTENTS);
    return fixtureFile;
  });

  const externalGuide = path.join(tmpDir, `workspace-storage-guide-${id}.md`);
  fs.writeFileSync(externalGuide, EXTERNAL_CONTENTS);
  const symlinkPath = path.join(checkoutRoot, "CLAUDE.md");
  fs.symlinkSync(externalGuide, symlinkPath);

  return {
    workspaceRoot,
    workspaceFile,
    markerFile,
    markerContents: marker,
    expectedBytes: Buffer.byteLength(WORKSPACE_CONTENTS) + Buffer.byteLength(marker),
    checkoutRoot,
    checkoutFilePaths,
    symlinkPath,
    externalGuide,
    cleanup: () => {
      fs.rmSync(workspaceRoot, { recursive: true, force: true });
      fs.rmSync(checkoutRoot, { recursive: true, force: true });
      fs.rmSync(externalGuide, { force: true });
    },
  };
}

export function assertWorkspaceStorageDiscoveryFixtureIsUnchanged(
  fixture: WorkspaceStorageDiscoveryFixture,
): void {
  if (fs.readFileSync(fixture.workspaceFile, "utf8") !== WORKSPACE_CONTENTS) {
    throw new Error("recognized workspace fixture changed");
  }
  if (fs.readFileSync(fixture.markerFile, "utf8") !== fixture.markerContents) {
    throw new Error("recognized workspace marker changed");
  }
  for (const filePath of fixture.checkoutFilePaths) {
    if (fs.readFileSync(filePath, "utf8") !== CHECKOUT_CONTENTS) {
      throw new Error(`unclassified checkout fixture changed: ${filePath}`);
    }
  }
  if (fs.readlinkSync(fixture.symlinkPath) !== fixture.externalGuide) {
    throw new Error("unclassified checkout symlink changed");
  }
  if (fs.readFileSync(fixture.externalGuide, "utf8") !== EXTERNAL_CONTENTS) {
    throw new Error("external symlink target changed");
  }
}
