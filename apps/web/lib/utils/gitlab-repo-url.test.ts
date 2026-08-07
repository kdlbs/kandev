import { describe, expect, it } from "vitest";
import { parseGitLabProjectUrl } from "./gitlab-repo-url";

const HOST = "https://gitlab.com";
const PROJECT = "acme/project";
const NESTED_PROJECT = "acme/team/project";
const WORKFLOWS_DIR = ".kandev/workflows";

describe("parseGitLabProjectUrl", () => {
  it("parses a plain project URL", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${PROJECT}`)).toEqual({ projectPath: PROJECT });
  });

  it("parses a nested subgroup project URL", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${NESTED_PROJECT}`)).toEqual({
      projectPath: NESTED_PROJECT,
    });
  });

  it("parses a self-managed host", () => {
    expect(parseGitLabProjectUrl(`https://gitlab.internal.example.com/${PROJECT}`)).toEqual({
      projectPath: PROJECT,
    });
  });

  it("tolerates trailing slash and missing scheme", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${PROJECT}/`)).toEqual({ projectPath: PROJECT });
    expect(parseGitLabProjectUrl(`gitlab.com/${PROJECT}`)).toEqual({ projectPath: PROJECT });
  });

  it("parses an SSH remote, including nested subgroups", () => {
    expect(parseGitLabProjectUrl(`git@gitlab.com:${PROJECT}.git`)).toEqual({
      projectPath: PROJECT,
    });
    expect(parseGitLabProjectUrl(`git@gitlab.internal.example.com:${NESTED_PROJECT}.git`)).toEqual({
      projectPath: NESTED_PROJECT,
    });
  });

  it("extracts branch and directory from a /-/tree/ link", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${NESTED_PROJECT}/-/tree/main/${WORKFLOWS_DIR}`)).toEqual(
      {
        projectPath: NESTED_PROJECT,
        branch: "main",
        path: WORKFLOWS_DIR,
      },
    );
  });

  it("extracts branch without path from a branch-root /-/tree/ link", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${PROJECT}/-/tree/develop`)).toEqual({
      projectPath: PROJECT,
      branch: "develop",
    });
  });

  it("resolves a /-/blob/ file link to the file's directory", () => {
    expect(
      parseGitLabProjectUrl(`${HOST}/${PROJECT}/-/blob/main/${WORKFLOWS_DIR}/dev.yml`),
    ).toEqual({
      projectPath: PROJECT,
      branch: "main",
      path: WORKFLOWS_DIR,
    });
  });

  it("parses a bare project-path ref with no scheme", () => {
    expect(parseGitLabProjectUrl(NESTED_PROJECT)).toEqual({ projectPath: NESTED_PROJECT });
    expect(parseGitLabProjectUrl(`${NESTED_PROJECT}/-/tree/main/${WORKFLOWS_DIR}`)).toEqual({
      projectPath: NESTED_PROJECT,
      branch: "main",
      path: WORKFLOWS_DIR,
    });
  });

  // A pasted HTTPS clone URL carries a .git suffix that must not leak into
  // the stored project path — a project path with .git on the end 404s
  // against GitLab, since no project is actually named that.
  it("strips a .git suffix from an HTTPS clone URL", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${PROJECT}.git`)).toEqual({ projectPath: PROJECT });
    expect(parseGitLabProjectUrl(`${HOST}/${NESTED_PROJECT}.git`)).toEqual({
      projectPath: NESTED_PROJECT,
    });
  });

  it("strips a .git suffix from a bare project-path ref", () => {
    expect(parseGitLabProjectUrl(`${PROJECT}.git`)).toEqual({ projectPath: PROJECT });
  });

  it("strips a bare-pasted host when it looks like a domain", () => {
    expect(parseGitLabProjectUrl(`gitlab.internal.example.com/${NESTED_PROJECT}`)).toEqual({
      projectPath: NESTED_PROJECT,
    });
  });

  it("rejects malformed input", () => {
    expect(parseGitLabProjectUrl(`${HOST}/only-namespace`)).toBeNull();
    expect(parseGitLabProjectUrl("not a url at all :::")).toBeNull();
    expect(parseGitLabProjectUrl("")).toBeNull();
    expect(parseGitLabProjectUrl("   ")).toBeNull();
  });

  it("rejects a bare host with only one following segment", () => {
    // Two segments where the first looks like a domain would otherwise be
    // stored as projectPath "gitlab.com/project" and every sync would 404.
    expect(parseGitLabProjectUrl("gitlab.com/only-project")).toBeNull();
  });

  it("returns null instead of throwing on malformed percent escapes", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${PROJECT}/-/tree/main/%`)).toBeNull();
  });

  it("decodes percent-encoded path segments", () => {
    expect(parseGitLabProjectUrl(`${HOST}/${PROJECT}/-/tree/main/my%20flows`)).toEqual({
      projectPath: PROJECT,
      branch: "main",
      path: "my flows",
    });
  });
});
