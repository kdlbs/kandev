import { expect, type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

export type KotlinTask = {
  taskId: string;
  sessionId: string;
  session: SessionPage;
  filePaths: string[];
};

type KotlinSeedData = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  repositoryId: string;
  agentProfileId: string;
};

type CreateKotlinTaskOptions = {
  title: string;
  fileCount?: number;
  fileContents?: string[];
  filePaths?: string[];
  extensions?: string[];
  executorProfileId?: string;
  navigate?: boolean;
  repositoryIds?: string[];
  repositoryDirectory?: string;
  push?: boolean;
};

export type FakeLspEvent = {
  event: string;
  pid: number;
  timestamp: number;
  id?: number;
  method?: string;
  params?: Record<string, unknown>;
  result?: unknown;
  cwd?: string;
  argv?: string[];
  signal?: string;
  reason?: string;
};

const FAKE_SERVER_SOURCE = path.resolve(__dirname, "../../fixtures/fake-lsp-server.mjs");

function fakeServerPath(backend: BackendContext): string {
  return path.join(backend.tmpDir, "bin", "kotlin-lsp");
}

function fakeServerLogPath(backend: BackendContext): string {
  return path.join(backend.tmpDir, "lsp-e2e-events.jsonl");
}

function crashModePath(backend: BackendContext): string {
  return path.join(backend.tmpDir, "lsp-e2e-crash-on-open");
}

export async function createKotlinTask(
  page: Page,
  apiClient: ApiClient,
  seedData: KotlinSeedData,
  backend: BackendContext,
  options: CreateKotlinTaskOptions,
): Promise<KotlinTask> {
  const fileCount = options.filePaths?.length ?? options.fileCount ?? 1;
  const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const extensions =
    options.extensions ??
    Array.from({ length: fileCount }, (_, index) => (index === 0 ? "kt" : "kts"));
  const filePaths =
    options.filePaths ??
    extensions.map((extension, index) =>
      index === 0 ? `Main-${suffix}.${extension}` : `Secondary-${suffix}-${index}.${extension}`,
    );
  const git = new GitHelper(
    path.join(backend.tmpDir, "repos", options.repositoryDirectory ?? "e2e-repo"),
    makeGitEnv(backend.tmpDir),
  );
  for (const [index, filePath] of filePaths.entries()) {
    const isTypeScript = filePath.endsWith(".ts");
    const defaultContent = isTypeScript
      ? [
          `export function greeting${index}(name: string): string {`,
          "  return `Hello, ${name}`;",
          "}",
          "",
        ].join("\n")
      : [
          `package e2e${index}`,
          "",
          `fun greeting${index}(name: String): String {`,
          '    return "Hello, $name"',
          "}",
          "",
        ].join("\n");
    git.createFile(filePath, options.fileContents?.[index] ?? defaultContent);
  }
  git.stageAll();
  git.commit(`add Kotlin LSP fixture ${suffix}`);
  if (options.push) git.exec("git push origin main");

  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    options.title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: options.repositoryIds ?? [seedData.repositoryId],
      executor_profile_id: options.executorProfileId,
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  const session = new SessionPage(page);
  if (options.navigate !== false) {
    await page.goto(`/t/${task.id}`);
    // Local Docker and SSH tasks can spend longer than the default 15 seconds
    // preparing their runtime before the chat panel mounts.
    await session.waitForLoad(45_000);
    await session.waitForChatIdle({ timeout: 45_000 });
  }

  return {
    taskId: task.id,
    sessionId: task.session_id,
    session,
    filePaths,
  };
}

export function expectedMonacoModelUri(documentUri: string, sessionId: string): string {
  const document = new URL(documentUri);
  if (document.protocol !== "file:" || document.search || document.hash || !sessionId) {
    throw new Error(`expected a clean file URI, received: ${documentUri}`);
  }
  const encodeToken = (value: string) => Buffer.from(value, "utf8").toString("hex");
  const sessionToken = `s-${encodeToken(sessionId)}`;
  let hostToken = "l";
  if (document.host) hostToken = `h-${encodeToken(document.host)}`;
  else if (/^\/[A-Za-z]:\//.test(document.pathname)) hostToken = "d";
  const strictEncode = (segment: string) =>
    encodeURIComponent(decodeURIComponent(segment)).replace(
      /[!'()*]/g,
      (character) => `%${character.codePointAt(0)!.toString(16).toUpperCase()}`,
    );
  const path = document.pathname.split("/").map(strictEncode).join("/");
  return `file:///__kandev_session_model__/${sessionToken}/${hostToken}${path}`;
}

export type LspDidOpenFrame = {
  sessionId: string;
  uri: string;
  text: string;
};

/** Capture clean protocol-level didOpen notifications sent over browser LSP sockets. */
export function attachLspDidOpenCapture(page: Page): LspDidOpenFrame[] {
  const frames: LspDidOpenFrame[] = [];
  page.on("websocket", (socket) => {
    const sessionMatch = /\/lsp\/([^?]+)/.exec(socket.url());
    if (!sessionMatch) return;
    const sessionId = decodeURIComponent(sessionMatch[1]);
    socket.on("framesent", (event) => {
      if (typeof event.payload !== "string" || !event.payload.includes("textDocument/didOpen")) {
        return;
      }
      try {
        const message = JSON.parse(event.payload) as {
          method?: string;
          params?: { textDocument?: { uri?: string; text?: string } };
        };
        const document = message.params?.textDocument;
        if (message.method === "textDocument/didOpen" && document?.uri && document.text) {
          frames.push({ sessionId, uri: document.uri, text: document.text });
        }
      } catch {
        // Ignore non-JSON frames and unrelated bridge traffic.
      }
    });
  });
  return frames;
}

export type MonacoModelSnapshot = {
  uri: string;
  pathname: string;
  search: string;
  hash: string;
  content: string;
};

/** Read browser-only, session-namespaced Monaco models for one file name. */
export async function readSessionModelSnapshots(
  page: Page,
  fileName: string,
): Promise<MonacoModelSnapshot[]> {
  return page.evaluate((targetFileName) => {
    const monaco = (
      window as typeof window & {
        monaco?: {
          editor: {
            getModels: () => Array<{
              getValue: () => string;
              uri: { toString: () => string };
            }>;
          };
        };
      }
    ).monaco;
    return (monaco?.editor.getModels() ?? []).flatMap((model) => {
      try {
        const uri = new URL(model.uri.toString());
        if (
          uri.protocol !== "file:" ||
          !decodeURIComponent(uri.pathname).endsWith(`/${targetFileName}`) ||
          !uri.pathname.startsWith("/__kandev_session_model__/s-")
        ) {
          return [];
        }
        return [
          {
            uri: uri.toString(),
            pathname: uri.pathname,
            search: uri.search,
            hash: uri.hash,
            content: model.getValue(),
          },
        ];
      } catch {
        return [];
      }
    });
  }, fileName);
}

export async function openDesktopFile(
  page: Page,
  session: SessionPage,
  filePath: string,
): Promise<void> {
  await session.clickTab("Files");
  await expect(session.files).toBeVisible({ timeout: 10_000 });
  const pathSegments = filePath.split("/");
  for (let index = 1; index < pathSegments.length; index++) {
    const ancestor = session.fileTreeNode(pathSegments.slice(0, index).join("/"));
    await expect(ancestor).toBeVisible({ timeout: 15_000 });
    if ((await ancestor.locator(".tabler-icon-chevron-right").count()) > 0) {
      await ancestor.click();
    }
  }
  const fileNode = session.fileTreeNode(filePath);
  await expect(fileNode).toBeVisible({ timeout: 15_000 });
  await fileNode.click();
  await expect(page.locator(".dv-default-tab", { hasText: path.basename(filePath) })).toBeVisible({
    timeout: 10_000,
  });
  await expect(page.locator(".monaco-editor:visible")).toBeVisible({ timeout: 15_000 });
}

export function removeFakeKotlinLsp(backend: BackendContext): void {
  const binary = fakeServerPath(backend);
  try {
    // The binary is test-owned and may not exist yet.
    fs.unlinkSync(binary);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
}

export function installFakeKotlinLsp(
  backend: BackendContext,
  options: { crashOnOpen?: boolean } = {},
): void {
  fs.mkdirSync(path.dirname(fakeServerPath(backend)), { recursive: true });
  fs.copyFileSync(FAKE_SERVER_SOURCE, fakeServerPath(backend));
  fs.chmodSync(fakeServerPath(backend), 0o755);
  fs.rmSync(fakeServerLogPath(backend), { force: true });
  fs.rmSync(crashModePath(backend), { force: true });
  if (options.crashOnOpen) fs.writeFileSync(crashModePath(backend), "1\n");
}

export function installAdditionalFakeLspBinary(backend: BackendContext, binaryName: string): void {
  const destination = path.join(backend.tmpDir, "bin", binaryName);
  fs.copyFileSync(FAKE_SERVER_SOURCE, destination);
  fs.chmodSync(destination, 0o755);
}

export function clearFakeKotlinLspModes(backend: BackendContext): void {
  fs.rmSync(crashModePath(backend), { force: true });
}

export function readFakeLspEvents(backend: BackendContext): FakeLspEvent[] {
  try {
    return fs
      .readFileSync(fakeServerLogPath(backend), "utf8")
      .split("\n")
      .filter(Boolean)
      .map((line) => JSON.parse(line) as FakeLspEvent);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return [];
    throw error;
  }
}

export async function expectFakeLspEvent(
  backend: BackendContext,
  predicate: (event: FakeLspEvent) => boolean,
  description: string,
  timeout = 15_000,
): Promise<FakeLspEvent> {
  let matched: FakeLspEvent | undefined;
  await expect
    .poll(
      () => {
        matched = readFakeLspEvents(backend).find(predicate);
        return matched !== undefined;
      },
      { message: `waiting for fake LSP event: ${description}`, timeout },
    )
    .toBe(true);
  return matched!;
}

export async function expectFakeLspMarkerCount(
  page: Page,
  expectedCount: number,
  timeout = 15_000,
): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate(() => {
          const monaco = (
            window as typeof window & {
              monaco?: {
                editor: {
                  getModelMarkers: (filter: Record<string, never>) => Array<{ source?: string }>;
                };
              };
            }
          ).monaco;
          return (
            monaco?.editor.getModelMarkers({}).filter((marker) => marker.source === "kandev-e2e")
              .length ?? 0
          );
        }),
      { message: `waiting for ${expectedCount} fake LSP Monaco marker(s)`, timeout },
    )
    .toBe(expectedCount);
}

export async function expectFakeLspMarkerMessages(
  page: Page,
  modelUri: string,
  expectedMessages: string[],
  timeout = 15_000,
): Promise<void> {
  await expect
    .poll(
      () =>
        page.evaluate((targetModelUri) => {
          const monaco = (
            window as typeof window & {
              monaco?: {
                editor: {
                  getModelMarkers: (filter: Record<string, never>) => Array<{
                    message: string;
                    resource: { toString: () => string };
                    source?: string;
                  }>;
                };
              };
            }
          ).monaco;
          return (monaco?.editor.getModelMarkers({}) ?? [])
            .filter(
              (marker) =>
                marker.source === "kandev-e2e" && marker.resource.toString() === targetModelUri,
            )
            .map((marker) => marker.message);
        }, modelUri),
      {
        message: `waiting for fake LSP Monaco markers on ${modelUri}`,
        timeout,
      },
    )
    .toEqual(expectedMessages);
}
