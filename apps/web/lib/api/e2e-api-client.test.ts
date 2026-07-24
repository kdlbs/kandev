import { afterEach, describe, expect, it, vi } from "vitest";

import { loadInterimSettingsInterlockToken } from "../../e2e/helpers/interim-settings-interlock";

describe("E2E interim settings interlock", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("loads the current boot token after boot rotation", async () => {
    let bootToken = "first-boot-token";
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string | URL) => {
        if (String(url).endsWith("/api/v1/app-state?path=%2Fsettings%2Fagents")) {
          return Response.json({ interimSettingsInterlockToken: bootToken });
        }
      }),
    );

    const firstToken = await loadInterimSettingsInterlockToken("http://backend.test");
    bootToken = "second-boot-token";
    const secondToken = await loadInterimSettingsInterlockToken("http://backend.test");

    expect([firstToken, secondToken]).toEqual(["first-boot-token", "second-boot-token"]);
  });
});
