import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import type { AddressInfo } from "node:net";
import { test as base, type WorkerInfo } from "./test-base";
import type { BackendContext } from "./backend";

type RecordedRequest = {
  method: string;
  path: string;
  body: Record<string, unknown>;
};

type MockRun = {
  id: string;
  agentId: string;
  status: string;
  createdAt: string;
  result: string;
  git: { branches: Array<{ repoUrl: string; branch: string; prUrl: string }> };
};

type HeldStream = { response: ServerResponse; run: MockRun };

export class CursorCloudMock {
  readonly requests: RecordedRequest[] = [];
  readonly prompts: string[] = [];
  private readonly server = createServer(
    (request, response) => void this.handle(request, response),
  );
  private port = 0;
  private agentId = "";
  private readonly runs = new Map<string, MockRun>();
  private submissionMode: "normal" | "unknown-followup" | "rate-limit" = "normal";
  private streamMode: "normal" | "retention-expired" | "disconnect" | "hold" = "normal";
  private authMode = "valid";
  private cancelPending = false;
  private readonly heldStreams = new Set<HeldStream>();

  get origin(): string {
    return `http://127.0.0.1:${this.port}`;
  }

  async listen(): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      this.server.once("error", reject);
      this.server.listen(0, "127.0.0.1", resolve);
    });
    this.port = (this.server.address() as AddressInfo).port;
  }

  async close(): Promise<void> {
    await new Promise<void>((resolve, reject) => {
      this.server.close((error) => (error ? reject(error) : resolve()));
    });
  }

  failNextSubmission(mode: "unknown-followup" | "rate-limit"): void {
    this.submissionMode = mode;
  }

  failNextStream(mode: "retention-expired" | "disconnect"): void {
    this.streamMode = mode;
  }

  expireCredentials(): void {
    this.authMode = "expired";
  }

  holdCancellationPending(): void {
    this.cancelPending = true;
  }

  releaseCancellation(): void {
    this.cancelPending = false;
    for (const run of this.runs.values()) {
      if (run.status === "RUNNING") run.status = "CANCELLED";
    }
    this.finishHeldStreams("CANCELLED");
  }

  holdNextStreamOpen(): void {
    this.streamMode = "hold";
  }

  finishHeldRun(): void {
    this.finishHeldStreams("FINISHED");
  }

  reset(): void {
    this.requests.length = 0;
    this.prompts.length = 0;
    this.agentId = "";
    this.runs.clear();
    this.heldStreams.clear();
    this.submissionMode = "normal";
    this.streamMode = "normal";
    this.authMode = "valid";
    this.cancelPending = false;
  }

  count(method: string, path: string): number {
    return this.requests.filter((request) => request.method === method && request.path === path)
      .length;
  }

  streamRequestCount(): number {
    return this.requests.filter(
      (request) => request.method === "GET" && request.path.endsWith("/stream"),
    ).length;
  }

  private async handle(request: IncomingMessage, response: ServerResponse): Promise<void> {
    const body = await readBody(request);
    const path = request.url ?? "/";
    this.requests.push({ method: request.method ?? "GET", path: path.split("?")[0]!, body });
    if (this.authMode === "expired") {
      return json(response, 401, { error: "expired credential", code: "unauthorized" });
    }
    if (this.handleCatalog(path, response)) return;
    if (this.handleCreateAgent(request, path, body, response)) return;
    this.handleAgentRequest(request, path, body, response);
  }

  private handleCatalog(path: string, response: ServerResponse): boolean {
    if (path === "/v1/models") {
      json(response, 200, {
        items: [{ id: "mock-cursor-model", displayName: "Mock Cursor Model" }],
      });
      return true;
    }
    if (path === "/v1/repositories") {
      json(response, 200, {
        items: [{ url: "https://github.com/mock-user/cursor-cloud-e2e", startingRef: "main" }],
      });
      return true;
    }
    return false;
  }

  private handleCreateAgent(
    request: IncomingMessage,
    path: string,
    body: Record<string, unknown>,
    response: ServerResponse,
  ): boolean {
    if (request.method !== "POST" || path !== "/v1/agents") return false;
    this.createAgent(body, response);
    return true;
  }

  private handleAgentRequest(
    request: IncomingMessage,
    path: string,
    body: Record<string, unknown>,
    response: ServerResponse,
  ): void {
    const segments = path.split("?")[0]!.split("/").filter(Boolean);
    if (!this.isCurrentAgentPath(segments)) {
      return json(response, 404, { error: "missing agent", code: "not_found" });
    }
    if (segments.length === 3 && request.method === "GET") {
      return json(response, 200, this.agent());
    }
    if (segments.length === 4 && segments[3] === "runs") {
      return this.handleRunsRequest(request, body, response);
    }
    if (segments.length === 5 && request.method === "GET") {
      const run = this.runs.get(segments[4]!);
      return json(response, run ? 200 : 404, run ?? { error: "missing run", code: "not_found" });
    }
    if (segments.length === 6) return this.handleRunAction(request, segments, response);
    return json(response, 404, { error: "missing route", code: "not_found" });
  }

  private isCurrentAgentPath(segments: string[]): boolean {
    return segments[0] === "v1" && segments[1] === "agents" && segments[2] === this.agentId;
  }

  private handleRunsRequest(
    request: IncomingMessage,
    body: Record<string, unknown>,
    response: ServerResponse,
  ): void {
    if (request.method === "GET") return json(response, 200, { items: [...this.runs.values()] });
    if (request.method === "POST") return this.createFollowup(body, response);
    return json(response, 404, { error: "missing route", code: "not_found" });
  }

  private handleRunAction(
    request: IncomingMessage,
    segments: string[],
    response: ServerResponse,
  ): void {
    const run = this.runs.get(segments[4]!);
    if (!run) return json(response, 404, { error: "missing run", code: "not_found" });
    if (segments[5] === "stream" && request.method === "GET") return this.stream(response, run);
    if (segments[5] === "cancel" && request.method === "POST") {
      if (!this.cancelPending) run.status = "CANCELLED";
      return json(response, 200, { id: run.id });
    }
    return json(response, 404, { error: "missing route", code: "not_found" });
  }

  private createAgent(body: Record<string, unknown>, response: ServerResponse): void {
    if (this.submissionMode === "rate-limit") {
      this.submissionMode = "normal";
      return json(response, 429, { error: "rate limited", code: "rate_limited" });
    }
    this.agentId = String(body.agentId ?? "");
    const prompt = body.prompt as { text?: string } | undefined;
    this.prompts.push(prompt?.text ?? "");
    const run = this.newRun("run-1");
    this.runs.set(run.id, run);
    json(response, 200, { agent: this.agent(), run });
  }

  private createFollowup(body: Record<string, unknown>, response: ServerResponse): void {
    const prompt = body.prompt as { text?: string } | undefined;
    this.prompts.push(prompt?.text ?? "");
    const run = this.newRun(`run-${this.runs.size + 1}`);
    if (this.submissionMode === "unknown-followup") {
      // The journal timestamp is written immediately before the provider call.
      // Keep this fixture run unambiguously after that boundary at millisecond
      // timestamp precision, even when the request completes in the same tick.
      run.createdAt = new Date(Date.now() + 1000).toISOString();
    }
    this.runs.set(run.id, run);
    if (this.submissionMode === "unknown-followup") {
      this.submissionMode = "normal";
      return json(response, 500, {
        error: "response lost after acceptance",
        code: "internal_error",
      });
    }
    json(response, 200, { run });
  }

  private stream(response: ServerResponse, run: MockRun): void {
    if (this.streamMode === "retention-expired") {
      this.streamMode = "normal";
      return json(response, 410, { error: "stream retention expired", code: "stream_expired" });
    }
    const mode = this.streamMode;
    this.streamMode = "normal";
    response.writeHead(200, {
      "Content-Type": "text/event-stream",
      "X-Cursor-Stream-Retention-Seconds": "3600",
    });
    response.write(`id: ${run.id}-1\nevent: assistant\ndata: {"text":"Remote work started"}\n\n`);
    if (mode === "disconnect") {
      response.destroy();
      return;
    }
    if (mode === "hold") {
      const held = { response, run };
      this.heldStreams.add(held);
      response.once("close", () => this.heldStreams.delete(held));
      return;
    }
    this.finishStream(response, run, "FINISHED");
  }

  private finishHeldStreams(status: "FINISHED" | "CANCELLED"): void {
    for (const held of this.heldStreams) {
      this.heldStreams.delete(held);
      this.finishStream(held.response, held.run, status);
    }
  }

  private finishStream(response: ServerResponse, run: MockRun, status: "FINISHED" | "CANCELLED") {
    run.status = status;
    response.write(
      `id: ${run.id}-2\nevent: result\ndata: ${JSON.stringify({ status, text: run.result })}\n\n`,
    );
    response.end();
  }

  private newRun(id: string): MockRun {
    return {
      id,
      agentId: this.agentId,
      status: "RUNNING",
      createdAt: new Date().toISOString(),
      result: id === "run-1" ? "Remote result is ready" : `Follow-up ${id} result is ready`,
      git: {
        branches: [
          {
            repoUrl: "https://github.com/mock-user/cursor-cloud-e2e",
            branch: "cursor/kandev-result",
            prUrl: "https://github.com/mock-user/cursor-cloud-e2e/pull/42",
          },
        ],
      },
    };
  }

  private agent() {
    const latestRunId = [...this.runs.keys()].at(-1) ?? "";
    return {
      id: this.agentId,
      name: "Kandev E2E task",
      status: "RUNNING",
      url: `https://cursor.com/agents/${this.agentId}`,
      latestRunId,
      repos: [{ url: "https://github.com/mock-user/cursor-cloud-e2e", startingRef: "main" }],
      workOnCurrentBranch: false,
      autoCreatePR: false,
    };
  }
}

async function readBody(request: IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) chunks.push(Buffer.from(chunk));
  if (chunks.length === 0) return {};
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as Record<string, unknown>;
}

function json(response: ServerResponse, status: number, value: unknown): void {
  response.writeHead(status, { "Content-Type": "application/json" });
  response.end(JSON.stringify(value));
}

type CursorCloudWorkerFixtures = { cursorCloud: CursorCloudMock };

export const test = base.extend<CursorCloudWorkerFixtures, CursorCloudWorkerFixtures>({
  cursorCloud: [
    async ({ backend }: { backend: BackendContext }, use, workerInfo: WorkerInfo) => {
      if (!workerInfo.project.name.startsWith("cursor-cloud")) {
        throw new Error("Cursor Cloud fixture can only run in its dedicated Playwright projects");
      }
      const mock = new CursorCloudMock();
      await mock.listen();
      const release = await backend.useEnv({
        KANDEV_FEATURES_CURSOR_CLOUD: "true",
        KANDEV_MOCK_CURSOR_CLOUD_BASE_URL: mock.origin,
      });
      try {
        await use(mock);
      } finally {
        await release();
        await mock.close();
      }
    },
    { scope: "worker" },
  ],
});

export { expect } from "@playwright/test";
