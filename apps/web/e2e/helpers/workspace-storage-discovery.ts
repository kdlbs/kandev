import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

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
