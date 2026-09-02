import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Turn } from "@/lib/types/http";
import { newestDurableTurnId } from "./pending-clarification";

// AC-10: this file and TestCurrentTurnResolutionFixture
// (apps/backend/internal/task/repository/sqlite/turn_authority_fixture_test.go)
// run the identical cases from one shared JSON fixture through the Go and
// TypeScript current-turn resolutions. Neither test may filter the shared
// cases; each may add its own. The path is resolved from this file via
// import.meta.url, not the process working directory, which under Vitest is
// apps/web rather than this file's directory. Building the path in two steps
// (dirname, then join) rather than `new URL(relative, import.meta.url)`
// avoids Vite's static asset-URL transform, which rewrites a
// literal-relative-plus-import.meta.url expression pointing outside the
// project root into an http://.../@fs/... dev-server URL instead of a real
// file: URL.
const FIXTURE_PATH = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../backend/internal/task/repository/sqlite/testdata/current_turn_resolution.json",
);

type FixtureTurn = {
  id: string;
  started_at: string;
  created_at: string;
  completed_at: string | null;
  metadata: Record<string, unknown>;
};

type FixtureCase = {
  name: string;
  turns: FixtureTurn[];
  expected_current_turn_id: string | null;
};

type Fixture = {
  cases: FixtureCase[];
};

// loadFixture SHALL fail the test (not skip) when the fixture is missing or
// unparseable - a fixture that silently stops being read is
// indistinguishable from two implementations that agree.
function loadFixture(): Fixture {
  const raw = readFileSync(FIXTURE_PATH, "utf8");
  const parsed = JSON.parse(raw) as Fixture;
  if (!parsed.cases?.length) {
    throw new Error(`${path.basename(FIXTURE_PATH)} has no cases`);
  }
  return parsed;
}

// completed_at: null in the fixture maps to an absent property, matching the
// wire contract (Turn.completed_at?: string); the production payload type is
// not widened to admit null.
function toTurn(fixtureTurn: FixtureTurn): Turn {
  const turn: Turn = {
    id: fixtureTurn.id,
    session_id: toSessionId("fixture-session"),
    task_id: toTaskId("fixture-task"),
    started_at: fixtureTurn.started_at,
    created_at: fixtureTurn.created_at,
    updated_at: fixtureTurn.created_at,
    metadata: fixtureTurn.metadata,
  };
  if (fixtureTurn.completed_at !== null) {
    turn.completed_at = fixtureTurn.completed_at;
  }
  return turn;
}

const fixture = loadFixture();

describe("newestDurableTurnId AC-10: shared fixture agreement with the Go resolution", () => {
  for (const testCase of fixture.cases) {
    it(testCase.name, () => {
      const turns = testCase.turns.map(toTurn);
      expect(newestDurableTurnId(turns)).toBe(testCase.expected_current_turn_id);
    });
  }
});
