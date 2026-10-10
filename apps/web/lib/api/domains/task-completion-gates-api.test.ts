import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  getTaskCompletionGate,
  listTaskCompletionGateHistory,
  setTaskCompletionCriteria,
  verifyTaskCompletionCriterion,
} from "./task-completion-gates-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

it("reads an encoded gate snapshot and its history", async () => {
  fetchSpy
    .mockResolvedValueOnce(
      new Response(
        JSON.stringify({ task_id: "task/1", revision: 2, criteria: [], blocked: false }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    )
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ items: [{ id: "history-1", action: "criteria_set" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

  await expect(getTaskCompletionGate("task/1", { baseUrl: API_BASE_URL })).resolves.toMatchObject({
    task_id: "task/1",
    revision: 2,
  });
  await expect(listTaskCompletionGateHistory("task/1", { baseUrl: API_BASE_URL })).resolves.toEqual(
    [{ id: "history-1", action: "criteria_set" }],
  );

  expect(fetchSpy.mock.calls.map(([url]) => url)).toEqual([
    `${API_BASE_URL}/api/v1/tasks/task%2F1/completion-gate`,
    `${API_BASE_URL}/api/v1/tasks/task%2F1/completion-gate/history`,
  ]);
});

it("writes revision-bound criteria and typed evidence", async () => {
  fetchSpy
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ task_id: "task-1", revision: 3 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    )
    .mockResolvedValueOnce(
      new Response(JSON.stringify({ task_id: "task-1", revision: 3, blocked: false }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

  await setTaskCompletionCriteria(
    "task-1",
    {
      expected_revision: 2,
      criteria: [
        {
          id: "checks",
          description: "Required checks pass",
          evidence_subject: { kind: "github_pr_head", id: "pr-1" },
        },
      ],
    },
    { baseUrl: API_BASE_URL },
  );
  await verifyTaskCompletionCriterion(
    "task-1",
    "checks/required",
    {
      expected_revision: 3,
      evidence: {
        subject: { kind: "github_pr_head", id: "pr-1", revision: "head-8" },
        summary: "Checks passed for the current head.",
      },
    },
    { baseUrl: API_BASE_URL },
  );

  expect(fetchSpy.mock.calls[0]).toMatchObject([
    `${API_BASE_URL}/api/v1/tasks/task-1/completion-gate/criteria`,
    {
      method: "PUT",
      body: JSON.stringify({
        expected_revision: 2,
        criteria: [
          {
            id: "checks",
            description: "Required checks pass",
            evidence_subject: { kind: "github_pr_head", id: "pr-1" },
          },
        ],
      }),
    },
  ]);
  expect(fetchSpy.mock.calls[1]).toMatchObject([
    `${API_BASE_URL}/api/v1/tasks/task-1/completion-gate/criteria/checks%2Frequired/evidence`,
    {
      method: "PUT",
      body: JSON.stringify({
        expected_revision: 3,
        evidence: {
          subject: { kind: "github_pr_head", id: "pr-1", revision: "head-8" },
          summary: "Checks passed for the current head.",
        },
      }),
    },
  ]);
});
