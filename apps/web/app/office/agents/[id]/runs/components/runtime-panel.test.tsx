import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { RuntimePanel } from "./runtime-panel";

afterEach(() => {
  cleanup();
});

describe("RuntimePanel", () => {
  it("renders enabled capabilities and skill snapshots", () => {
    render(
      <RuntimePanel
        runtime={{
          capabilities: { create_agent: true, delete_skills: false, post_comment: true },
          input_snapshot: {},
          session_id: "sess-1",
          skills: [
            {
              skill_id: "skill-1",
              version: "1",
              content_hash: "abc",
              materialized_path: "/tmp/skills",
            },
          ],
        }}
      />,
    );

    expect(screen.getByTestId("runtime-panel")).toBeTruthy();
    expect(screen.getByTestId("runtime-capabilities").textContent).toContain("create_agent");
    expect(screen.getByTestId("runtime-capabilities").textContent).not.toContain("delete_skills");
    expect(screen.getByTestId("runtime-skills").textContent).toContain("skill-1");
    expect(screen.getByTestId("runtime-skills").textContent).toContain("hash abc");
  });

  it("prefers captured skill labels and shows an explicit fallback when absent", () => {
    render(
      <RuntimePanel
        runtime={{
          capabilities: {},
          input_snapshot: {},
          skills: [
            {
              skill_id: "skill-renamed",
              display_name: "Planning",
              slug: "planning",
              version: "2",
              content_hash: "hash-2",
              materialized_path: "/tmp/skills/planning",
            },
            {
              skill_id: "skill-deleted",
              version: "1",
              content_hash: "hash-1",
              materialized_path: "/tmp/skills/deleted",
            },
          ],
        }}
      />,
    );

    const skills = screen.getByTestId("runtime-skills");
    expect(skills.textContent).toContain("Planning");
    expect(skills.textContent).toContain("skill-renamed");
    expect(skills.textContent).toMatch(/skill unavailable/i);
  });
});
