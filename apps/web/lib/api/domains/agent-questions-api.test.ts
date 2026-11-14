import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

const mocks = vi.hoisted(() => ({ fetchJson: vi.fn() }));

vi.mock("../client", () => ({ fetchJson: mocks.fetchJson }));

import { getAgentQuestion } from "./agent-questions-api";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("getAgentQuestion", () => {
  beforeEach(() => {
    mocks.fetchJson.mockReset();
  });

  it("shares an in-flight detail read for the same question", async () => {
    const response = deferred<ClarificationInboxAsyncQuestion>();
    const question = { question_id: "question-read-dedupe" } as ClarificationInboxAsyncQuestion;
    mocks.fetchJson.mockReturnValue(response.promise);

    const first = getAgentQuestion(question.question_id);
    const second = getAgentQuestion(question.question_id);
    expect(mocks.fetchJson).toHaveBeenCalledTimes(1);
    expect(mocks.fetchJson).toHaveBeenCalledWith("/api/v1/agent-questions/question-read-dedupe");

    response.resolve(question);
    await expect(first).resolves.toBe(question);
    await expect(second).resolves.toBe(question);
  });
});
