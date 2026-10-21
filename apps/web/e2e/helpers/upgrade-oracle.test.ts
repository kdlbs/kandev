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

it("preserves original message identities, contents and order while allowing appended turns", async () => {
  const { assertHistoricalMessagesPreserved } = await import("./upgrade-oracle");
  const messages = [
    { id: "user", content: "original request" },
    { id: "agent", content: "original response" },
  ];
  const snapshot = (history: unknown[]) => ({
    tasks: [{ id: "task", sessions: [{ id: "session", messages: history }] }],
  });
  expect(() =>
    assertHistoricalMessagesPreserved(
      snapshot(messages),
      snapshot([...messages, { id: "new", content: "follow-up" }]),
    ),
  ).not.toThrow();
  for (const history of [
    messages.slice(1),
    [{ ...messages[0], content: "changed" }, messages[1]],
    [messages[0], { ...messages[1], id: "copied" }],
    [...messages].reverse(),
    [...messages, messages[0]],
  ])
    expect(() => assertHistoricalMessagesPreserved(snapshot(messages), snapshot(history))).toThrow(
      /preservation/,
    );
  expect(() => assertHistoricalMessagesPreserved(snapshot(messages), { tasks: [] })).toThrow(
    /preservation/,
  );
});
