import type { Page, Route } from "@playwright/test";
import { PLUGIN_ID } from "./plugin-fixture";

const HUMAN_RESPONSES = "human_responses";
const LINK_CODE = "ABCD-EFGH-JKMN";

export type MockIdentityLink = {
  id: string;
  provider: string;
  subject: string;
  display_name: string;
  user_id: string;
  user_display_name: string;
  created_at: string;
};

export function mockIdentityLink(id: string, displayName: string): MockIdentityLink {
  return {
    id,
    provider: "discord",
    subject: `subject-${id}`,
    display_name: displayName,
    user_id: "user-1",
    user_display_name: "Ada",
    created_at: "2026-11-22T10:00:00Z",
  };
}

type PluginRecordBody = { capabilities?: { api_write?: string[] } } & Record<string, unknown>;

function declareHumanResponses(record: PluginRecordBody): PluginRecordBody {
  const apiWrite = record.capabilities?.api_write ?? [];
  if (apiWrite.includes(HUMAN_RESPONSES)) return record;
  return {
    ...record,
    capabilities: { ...record.capabilities, api_write: [...apiWrite, HUMAN_RESPONSES] },
  };
}

async function patchPluginRecord(route: Route): Promise<void> {
  const response = await route.fetch();
  const body = (await response.json()) as PluginRecordBody & { plugins?: PluginRecordBody[] };
  if (Array.isArray(body.plugins)) {
    body.plugins = body.plugins.map((plugin) =>
      plugin.id === PLUGIN_ID ? declareHumanResponses(plugin) : plugin,
    );
    await route.fulfill({ response, json: body });
    return;
  }
  await route.fulfill({ response, json: declareHumanResponses(body) });
}

/**
 * The shared fixture plugin does not declare `human_responses`, so the host
 * would refuse link codes for it. These routes present it as a plugin that
 * does, and serve the identity-link endpoints from the returned `links` array,
 * so a spec can drive the "Linked chat accounts" card end to end.
 */
export async function mockHumanResponsesPlugin(
  page: Page,
  links: MockIdentityLink[],
): Promise<{ links: MockIdentityLink[] }> {
  const state = { links };
  await page.route(new RegExp(`/api/plugins(/${PLUGIN_ID})?(\\?.*)?$`), async (route) => {
    if (route.request().method() !== "GET") return route.fallback();
    return patchPluginRecord(route);
  });
  await page.route(new RegExp(`/api/plugins/${PLUGIN_ID}/identity-links`), async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === "POST" && url.pathname.endsWith("/codes")) {
      const expires = new Date(Date.now() + 10 * 60_000).toISOString();
      return route.fulfill({ status: 200, json: { code: LINK_CODE, expires_at: expires } });
    }
    if (request.method() === "DELETE") {
      const linkId = decodeURIComponent(url.pathname.split("/").pop() ?? "");
      state.links = state.links.filter((link) => link.id !== linkId);
      return route.fulfill({ status: 204 });
    }
    return route.fulfill({ status: 200, json: { links: state.links } });
  });
  return state;
}

export const IDENTITY_LINK_CODE = LINK_CODE;
