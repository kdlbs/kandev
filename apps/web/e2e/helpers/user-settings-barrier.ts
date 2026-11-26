import type { Page, Route } from "@playwright/test";
import type { UserSettingsUpdatePayload } from "@/lib/types/http-user-settings";

export async function holdSidebarSettingsWrite(
  page: Page,
  options: { commit?: boolean; responseStatus?: number } = {},
) {
  const arrived = Promise.withResolvers<void>();
  const released = Promise.withResolvers<void>();
  const finished = Promise.withResolvers<void>();
  let held = false;
  let writes = 0;
  let body: UserSettingsUpdatePayload["sidebar_view_state"];
  const handler = async (route: Route) => {
    const request = route.request();
    const mutation = request.method() === "PATCH" && request.postDataJSON()?.sidebar_view_state;
    if (!mutation) return route.continue();
    writes++;
    if (held) return route.continue();
    held = true;
    body = mutation;
    try {
      const committed = options.commit ? await route.fetch() : undefined;
      arrived.resolve();
      await released.promise;
      if (options.responseStatus)
        await route.fulfill({
          status: options.responseStatus,
          json: { error: "Proxy rejected the committed response" },
        });
      else if (committed) await route.fulfill({ response: committed });
      else await route.continue();
    } finally {
      finished.resolve();
    }
  };
  await page.route("**/api/v1/user/settings", handler);
  return {
    arrived: arrived.promise,
    release: () => released.resolve(),
    get body() {
      return body;
    },
    get writes() {
      return writes;
    },
    dispose: async () => {
      released.resolve();
      if (held) await finished.promise;
      await page.unroute("**/api/v1/user/settings", handler);
    },
  };
}
