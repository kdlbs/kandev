import { expect, it } from "vitest";
import { assertPreserved } from "./upgrade-oracle";

it("detects changed, missing, duplicated and extra historical records", () => {
  const old = [{ id: "a", content: "old text", created_at: "2026-10-01" }];
  expect(() => assertPreserved(old, structuredClone(old))).not.toThrow();
  for (const actual of [
    [],
    [{ ...old[0], content: "changed" }],
    [old[0], old[0]],
    [...old, { ...old[0], id: "b" }],
    [{ ...old[0], created_at: "2026-10-02" }],
  ]) {
    expect(() => assertPreserved(old, actual)).toThrow(/preservation/);
  }
});
it("reports the field path rather than silently dropping changed relationships", () => {
  expect(() =>
    assertPreserved({ session: { task_id: "old" } }, { session: { task_id: "other" } }),
  ).toThrow(/session.task_id/);
});
