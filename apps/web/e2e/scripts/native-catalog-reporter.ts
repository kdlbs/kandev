import path from "node:path";
import type { FullConfig, Reporter, Suite, TestError } from "@playwright/test/reporter";
import type { CatalogCase } from "./native-catalog";

export default class NativeCatalogReporter implements Reporter {
  private cases: CatalogCase[] = [];
  private errors: TestError[] = [];

  onBegin(_config: FullConfig, suite: Suite): void {
    for (const test of suite.allTests()) {
      let owner: Suite | undefined = test.parent;
      while (owner && owner.type !== "file") owner = owner.parent;
      const project = test.parent.project()?.name;
      if (!owner?.location || !project) throw new Error("Native test has no owning file/project");
      this.cases.push({
        project,
        file: path.relative(path.resolve("e2e"), owner.location.file).split(path.sep).join("/"),
        testId: test.id,
      });
    }
  }

  onError(error: TestError): void {
    this.errors.push(error);
  }
  onEnd(): void {
    process.stdout.write(JSON.stringify({ cases: this.cases, errors: this.errors }));
  }
  printsToStdio(): boolean {
    return true;
  }
}
