import { afterEach, expect, it, vi } from "vitest";
import { getThemeDefinition, prepareThemeFile, commitThemeReview } from "./themes-api";
vi.mock("@/lib/config", () => ({ getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }) }));
afterEach(() => vi.unstubAllGlobals());
const descriptor = {
  package_id: "test",
  package_name: "Test",
  key: "night",
  label: "Night",
  base: "dark" as const,
  version: "1.0.0",
  digest: "aabb",
};
it("requires validated content with the selected base", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({ schema_version: 1, base: "light" })))
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({ schema_version: 1, base: "dark", ui: { background: "url(evil)" } }),
      ),
    );
  vi.stubGlobal("fetch", fetch);
  await expect(getThemeDefinition(descriptor)).rejects.toThrow();
  await expect(getThemeDefinition(descriptor)).rejects.toThrow();
  expect(String(fetch.mock.calls[0][0])).toContain("/themes/test/night?digest=aabb");
});
it("uploads exact file bytes and commits only the prepared review identity", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "review-1" })));
  vi.stubGlobal("fetch", fetch);
  const file = new File(['{"type":"dark"}'], "theme.json");
  await prepareThemeFile(file);
  const form = fetch.mock.calls[0][1].body as FormData;
  expect(form.get("package")).toBe(file);
  fetch.mockResolvedValue(new Response(JSON.stringify({ plugin: { id: "test" } })));
  await commitThemeReview("review-1");
  expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({ id: "review-1" });
});
