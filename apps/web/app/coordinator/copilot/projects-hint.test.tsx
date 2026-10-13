import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, o?: { list?: string; count?: number }) => {
      const fixed: Record<string, string> = {
        "coordinator:copilotProjectsLine": `P: ${o?.list}`,
        "coordinator:copilotProjectsMore": `+${o?.count}`,
        "coordinator:copilotProjectsAll": "ALL",
        "coordinator:copilotProjectsNoRepository": "NOREPO",
      };
      return fixed[key];
    },
  }),
}));

import { ProjectsHint } from "./projects-hint";

afterEach(() => cleanup());

const line = () => screen.queryByTestId("workspace-copilot-projects")?.textContent ?? null;

describe("ProjectsHint", () => {
  it("renders nothing without projects", () => {
    render(<ProjectsHint projects={undefined} />);
    expect(line()).toBeNull();
  });

  it("says all under scope all", () => {
    render(<ProjectsHint projects={{ scope: "all" }} />);
    expect(line()).toBe("P: ALL");
  });

  it("joins up to five names, then the remainder, then no-repository last", () => {
    render(
      <ProjectsHint
        projects={{
          scope: "selected",
          repository_ids: ["r"],
          include_no_repository: true,
          names: ["a", "b", "c", "d", "e", "f", "g"],
        }}
      />,
    );
    expect(line()).toBe("P: a, b, c, d, e +2 NOREPO");
  });

  it("shows only the no-repository text for empty names with the toggle on", () => {
    render(
      <ProjectsHint
        projects={{ scope: "selected", repository_ids: [], include_no_repository: true, names: [] }}
      />,
    );
    expect(line()).toBe("P: NOREPO");
  });

  it("is omitted when the names are absent, whatever the toggle", () => {
    render(
      <ProjectsHint
        projects={{ scope: "selected", repository_ids: null, include_no_repository: true }}
      />,
    );
    expect(line()).toBeNull();
  });

  it("shows nothing for empty names with the toggle off", () => {
    render(
      <ProjectsHint
        projects={{
          scope: "selected",
          repository_ids: [],
          include_no_repository: false,
          names: [],
        }}
      />,
    );
    expect(line()).toBeNull();
  });
});
