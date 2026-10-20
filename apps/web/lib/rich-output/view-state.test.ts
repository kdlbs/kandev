import { expect, it } from "vitest";
import { getRichOutputViewStore } from "./view-state";

const FIELD = "/file/expanded";
const IDENTITY = "user/session/message";

it("retains host view preferences for one owner and report, and isolates identities", () => {
  const owner = {};
  const store = getRichOutputViewStore(owner, IDENTITY);
  store.set(FIELD, true);
  expect(getRichOutputViewStore(owner, IDENTITY).get(FIELD)).toBe(true);
  expect(getRichOutputViewStore({}, IDENTITY).get(FIELD)).toBeUndefined();
  expect(getRichOutputViewStore(owner, "other/session/message").get(FIELD)).toBeUndefined();
});
it("bounds retained report preferences and evicts the least recently used report", () => {
  const owner = {};
  const first = getRichOutputViewStore(owner, "0");
  for (let i = 1; i <= 32; i++) getRichOutputViewStore(owner, String(i));
  expect(getRichOutputViewStore(owner, "0")).not.toBe(first);
});
