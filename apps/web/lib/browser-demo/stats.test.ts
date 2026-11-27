import { describe, expect, it } from "vitest";
import type { GlobalStatsDTO, RepositoryStatsDTO } from "@/lib/types/http";
import { createDemoState } from "./scenario";
import { createDemoStats } from "./stats";

describe("browser demo statistics", () => {
  it("returns populated global statistics derived from the scenario", () => {
    const state = createDemoState();
    const stats = createDemoStats("global", state) as GlobalStatsDTO;

    expect(stats.total_tasks).toBe(state.tasks.length);
    expect(stats.total_sessions).toBe(state.sessions.length);
    expect(stats.total_messages).toBeGreaterThan(0);
  });

  it("returns repository activity and rejects unknown sections", () => {
    const repositories = createDemoStats("repositories", createDemoState()) as RepositoryStatsDTO[];

    expect(repositories[0]).toMatchObject({
      repository_name: "acme-web",
      total_commits: 18,
    });
    expect(createDemoStats("unknown", createDemoState())).toBeUndefined();
  });
});
