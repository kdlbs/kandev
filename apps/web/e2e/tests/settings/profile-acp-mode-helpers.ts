import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { expect, type Page, type Response } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

const AGENT_SLUG = "e2e-profile-mode-acp-agent";
const AGENT_DISPLAY_NAME = "E2E Profile Mode ACP Agent";
const PROFILE_NAME = "E2E Profile Mode ACP Profile";
const FIXTURE_COMMAND = path.resolve(
  __dirname,
  "../../helpers/profile-mode-config-acp-fixture.mjs",
);

export type ProfileModeFixture = {
  agentName: string;
  profileId: string;
  observationFile: string;
  cleanup: () => Promise<void>;
};

export async function createProfileModeFixture(
  apiClient: ApiClient,
  initialMode: string,
): Promise<ProfileModeFixture> {
  const fixtureDir = await mkdtemp(path.join(os.tmpdir(), "kandev-profile-mode-"));
  const observationFile = path.join(fixtureDir, "first-prompts.jsonl");
  let agentName = AGENT_SLUG;

  try {
    const agent = await apiClient.createCustomTUIAgent({
      display_name: AGENT_DISPLAY_NAME,
      command: "node",
      command_args: [FIXTURE_COMMAND, observationFile],
      protocol: "acp",
    });
    agentName = agent.name;
    const profile = await apiClient.createAgentProfile(agent.id, PROFILE_NAME, {
      model: "fixture-model",
      mode: initialMode,
      cli_passthrough: false,
    });

    return {
      agentName,
      profileId: profile.id,
      observationFile,
      cleanup: async () => {
        await apiClient.deleteAgentProfile(profile.id, true);
        await apiClient.deleteCustomAgentByName(AGENT_SLUG);
        await rm(fixtureDir, { recursive: true, force: true });
      },
    };
  } catch (error) {
    await apiClient.deleteCustomAgentByName(agentName);
    await rm(fixtureDir, { recursive: true, force: true });
    throw error;
  }
}

export function resolvesConfigMode(expectedMode: string) {
  return async (response: Response): Promise<boolean> => {
    if (response.status() !== 200) return false;
    const request = response.request().postDataJSON() as { mode?: string };
    if (request.mode !== expectedMode) return false;

    try {
      const body = (await response.json()) as {
        status?: string;
        config_options?: Array<{ category?: string; current_value?: string }>;
      };
      return (
        body.status === "ok" &&
        body.config_options?.some(
          (option) => option.category === "mode" && option.current_value === expectedMode,
        ) === true
      );
    } catch {
      return false;
    }
  };
}

export async function expectFirstPromptMode(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  fixture: ProfileModeFixture,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "ACP config mode profile launch",
    fixture.profileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("ACP profile mode task has no session_id");

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.expectChatResponseVisible("ACP profile mode fixture response", 0, {
    timeout: 60_000,
  });
  await session.waitForChatIdle({ timeout: 30_000 });

  const records = (await readFile(fixture.observationFile, "utf8"))
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line) as { mode: string; prompt: unknown });
  expect(records).toHaveLength(1);
  expect(records[0].mode).toBe("session");
  expect(records[0].prompt).toBeTruthy();
}
