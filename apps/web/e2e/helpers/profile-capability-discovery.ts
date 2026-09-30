import fs from "node:fs";
import path from "node:path";
import type { Page, Request } from "@playwright/test";
import type { AgentProfile } from "../../lib/types/http";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";

export type ProfileDiscoveryRequest = {
  url: string;
  body: Record<string, unknown>;
  responseStatus?: number;
  failure?: string;
};

export type ProfileProbeEvidence = {
  wrapper: boolean;
  command?: string;
  env_catalog?: string;
  cli_catalog?: string;
  profile_args?: string[];
};

export function observeProfileDiscoveryRequests(page: Page): ProfileDiscoveryRequest[] {
  const requests: ProfileDiscoveryRequest[] = [];
  const observedByRequest = new Map<Request, ProfileDiscoveryRequest>();
  page.on("request", (request) => {
    if (!request.url().includes("/api/v1/agent-models/mock-agent/")) return;
    const postData = request.postData();
    if (!postData) return;
    try {
      const body = JSON.parse(postData) as Record<string, unknown>;
      const observed = { url: request.url(), body };
      requests.push(observed);
      observedByRequest.set(request, observed);
    } catch {
      // Other request payloads do not contribute to the probe evidence.
    }
  });
  page.on("response", (response) => {
    const observed = observedByRequest.get(response.request());
    if (!observed) return;
    observed.responseStatus = response.status();
  });
  page.on("requestfailed", (request) => {
    const observed = observedByRequest.get(request);
    if (observed) observed.failure = request.failure()?.errorText ?? "request failed";
  });
  return requests;
}

export async function createProfileWithCatalog(
  apiClient: ApiClient,
  backend: BackendContext,
  agentId: string,
  name: string,
  catalog: string,
): Promise<{ profile: AgentProfile; evidencePath: string }> {
  const evidencePath = path.join(
    backend.tmpDir,
    `profile-discovery-${name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}.jsonl`,
  );
  fs.rmSync(evidencePath, { force: true });
  const profile = await apiClient.createAgentProfile(agentId, name, {
    model: "mock-fast",
    env_vars: [
      { key: "MOCK_AGENT_PROFILE_CATALOG", value: catalog },
      { key: "MOCK_AGENT_PROFILE_EVIDENCE_FILE", value: evidencePath },
    ],
    cli_flags: [
      {
        description: "Profile catalog fixture",
        flag: `--profile-catalog=${catalog}`,
        enabled: true,
      },
    ],
    command_prefix: "mock-agent --profile-probe-wrapper",
  });
  return { profile, evidencePath };
}

export function readProfileProbeEvidence(pathname: string): ProfileProbeEvidence[] {
  if (!fs.existsSync(pathname)) return [];
  return fs
    .readFileSync(pathname, "utf8")
    .split("\n")
    .filter(Boolean)
    .map((line) => JSON.parse(line) as ProfileProbeEvidence);
}
