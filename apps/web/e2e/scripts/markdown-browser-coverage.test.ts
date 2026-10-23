import fs from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { parse } from "yaml";
import config from "../playwright.config";

const workflow = parse(
  fs.readFileSync(path.resolve(__dirname, "../../../../.github/workflows/e2e-tests.yml"), "utf8"),
);
const projects = ["markdown-firefox", "markdown-webkit", "markdown-mobile-webkit"];

describe("Markdown browser coverage", () => {
  // @covers AC-UI-MARKDOWN-FILE-EDITING-003.11
  it("runs real Firefox and WebKit only for their Markdown surfaces", () => {
    const cases = [
      ["markdown-firefox", "firefox", "markdown-file-editing.spec.ts"],
      ["markdown-webkit", "webkit", "markdown-file-editing.spec.ts"],
      ["markdown-mobile-webkit", "webkit", "mobile-markdown-file-editing.spec.ts"],
    ];
    expect(config.workers).toBe(1);
    for (const [name, browser, file] of cases) {
      const project = config.projects?.find((item) => item.name === name);
      expect(project?.use?.defaultBrowserType).toBe(browser);
      const matcher = project?.testMatch as RegExp;
      expect(matcher.test(`/task/${file}`)).toBe(true);
      expect(matcher.test("/task/task-create.spec.ts")).toBe(false);
      const other = file.startsWith("mobile-")
        ? "markdown-file-editing.spec.ts"
        : "mobile-markdown-file-editing.spec.ts";
      expect(matcher.test(`/task/${other}`)).toBe(false);
    }
  });

  it("installs native browsers and runs guarded projects against identified artifacts", () => {
    const job = workflow.jobs["e2e-markdown-browsers"];
    expect(job).toBeDefined();
    expect(job.needs).toEqual(["changes", "build"]);
    expect(job.strategy.matrix.project).toEqual(projects);
    const steps = job.steps as Array<{ run?: string; with?: { name?: string } }>;
    expect(
      steps.some((step) => step.run === "pnpm exec playwright install --with-deps firefox webkit"),
    ).toBe(true);
    expect(
      steps.some(
        (step) => step.run === 'pnpm e2e:raw --project="${{ matrix.project }}" --workers=1',
      ),
    ).toBe(true);
    for (const artifact of ["e2e-backend-build", "e2e-web-build", "e2e-plugin-package"]) {
      expect(steps.some((step) => step.with?.name === artifact)).toBe(true);
    }
    const execution = job.steps.find((step: { run?: string }) =>
      step.run?.startsWith("pnpm e2e:raw"),
    );
    expect(execution.env.KANDEV_E2E_BUILD_IDENTITY).toContain("e2e-build-identity.json");
    expect(execution.env.KANDEV_E2E_EXPECTED_SOURCE_REVISION).toBe("${{ github.sha }}");
    const homeOwnership = steps.findIndex(
      (step) => step.run === 'chown "$(id -u):$(id -g)" "$HOME"',
    );
    expect(homeOwnership).toBeGreaterThanOrEqual(0);
    expect(homeOwnership).toBeLessThan(job.steps.indexOf(execution));
  });

  it("requires browser success while keeping supplemental reports outside the native catalog", () => {
    expect(workflow.jobs["e2e-report"].needs).toContain("e2e-markdown-browsers");
    expect(workflow.jobs["e2e-gate"]).toBeUndefined();
    const artifacts = workflow.jobs["e2e-markdown-browsers"].steps
      .filter((step: { uses?: string }) => step.uses?.startsWith("actions/upload-artifact@"))
      .map((step: { with: { name: string } }) => step.with.name);
    expect(artifacts).toContain("markdown-browser-blob-${{ matrix.project }}");
    expect(artifacts.some((name: string) => name.startsWith("blob-report-"))).toBe(false);
    const steps = workflow.jobs["e2e-report"].steps as Array<{
      env?: Record<string, string>;
      run?: string;
    }>;
    expect(
      steps.some(
        (step) =>
          step.env?.MARKDOWN_BROWSER_RESULT === "${{ needs['e2e-markdown-browsers'].result }}",
      ),
    ).toBe(true);
    expect(
      steps.some((step) => step.run?.includes('"markdown-browsers:${MARKDOWN_BROWSER_RESULT}"')),
    ).toBe(true);
  });
});
