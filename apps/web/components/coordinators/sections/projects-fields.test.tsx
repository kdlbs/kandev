import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProjectsFields, type ProjectsFieldsProps } from "./projects-fields";

const SET_TOGGLE = "watches-project-toggle-set-1";

afterEach(cleanup);

const sets = [
  { kind: "repository_set" as const, id: "set-1", name: "Payments", repositoryCount: 2 },
];
const loose = [{ kind: "repository" as const, id: "repo-d", name: "Delta" }];

function renderFields(overrides: Partial<ProjectsFieldsProps> = {}) {
  const onChange = vi.fn();
  const onRetry = vi.fn();
  render(
    <ProjectsFields
      projects={{
        scope: "selected",
        entries: [{ kind: "repository", id: "repo-d" }],
        includeNoRepository: false,
      }}
      sets={sets}
      loose={loose}
      status="ready"
      onRetry={onRetry}
      canManage
      onChange={onChange}
      {...overrides}
    />,
  );
  return { onChange, onRetry };
}

describe("ProjectsFields", () => {
  it("offers a retry when the projects failed to load", () => {
    const { onRetry } = renderFields({ status: "error", sets: [], loose: [] });
    const failed = screen.getByTestId("watches-projects-failed");
    fireEvent.click(failed.querySelector("button")!);
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["the projects failed to load", { status: "error" as const, sets: [], loose: [] }],
    ["the workspace has no projects", { status: "ready" as const, sets: [], loose: [] }],
  ])("keeps every project watched when %s", (_label, over) => {
    const { onChange } = renderFields({
      ...over,
      projects: { scope: "all", entries: [], includeNoRepository: false },
    });
    const all = screen.getByTestId("watches-projects-all") as HTMLButtonElement;
    expect(all.disabled).toBe(true);
    fireEvent.click(all);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("shows the server error and the keep-one hint for an empty selected draft", () => {
    renderFields({
      projects: { scope: "selected", entries: [], includeNoRepository: false },
      errorMessage: "Choose at least one project.",
    });
    expect(screen.getByTestId("watches-projects-error").textContent).toBe(
      "Choose at least one project.",
    );
    expect(screen.getByTestId("watches-keep-one-project")).toBeTruthy();
  });

  it("puts a project in and takes one out through onChange", () => {
    const { onChange } = renderFields();
    fireEvent.click(screen.getByTestId(SET_TOGGLE));
    expect(onChange).toHaveBeenLastCalledWith({
      scope: "selected",
      entries: [
        { kind: "repository", id: "repo-d" },
        { kind: "repository_set", id: "set-1" },
      ],
      includeNoRepository: false,
    });
    fireEvent.click(screen.getByTestId("watches-project-toggle-repo-d"));
    expect(onChange).toHaveBeenLastCalledWith({
      scope: "selected",
      entries: [],
      includeNoRepository: false,
    });
  });

  it("disables every control for a reader", () => {
    const { onChange } = renderFields({ canManage: false });
    for (const id of ["watches-projects-all", "watches-projects-no-repo"]) {
      expect((screen.getByTestId(id) as HTMLButtonElement).disabled).toBe(true);
    }
    for (const id of [SET_TOGGLE, "watches-project-toggle-repo-d"]) {
      expect((screen.getByTestId(id) as HTMLButtonElement).disabled).toBe(true);
    }
    fireEvent.click(screen.getByTestId(SET_TOGGLE));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("hides the project list under watch-every-project", () => {
    renderFields({ projects: { scope: "all", entries: [], includeNoRepository: false } });
    expect(screen.queryByTestId("watches-project-list")).toBeNull();
    expect(screen.queryByTestId("watches-projects-no-repo")).toBeNull();
  });

  it("disables adding at the 50 project cap", () => {
    const many = Array.from({ length: 50 }, (_, i) => ({
      kind: "repository" as const,
      id: `r${i}`,
    }));
    renderFields({
      projects: { scope: "selected", entries: many, includeNoRepository: false },
    });
    expect((screen.getByTestId(SET_TOGGLE) as HTMLButtonElement).disabled).toBe(true);
  });
});
