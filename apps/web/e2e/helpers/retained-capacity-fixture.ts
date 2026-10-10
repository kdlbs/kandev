import { execFile } from "node:child_process";
import { promisify } from "node:util";
import path from "node:path";
import { expect } from "@playwright/test";

const execute = promisify(execFile);

export async function retainedCapacityFixture(
  fixtureRoot: string,
  sessionId: string,
  action: "fill" | "assert_pruned",
) {
  expect(path.basename(fixtureRoot)).toMatch(/^kandev-e2e-/);
  const result = await execute(
    path.resolve(
      process.cwd(),
      `../backend/bin/e2e-delivery-fixture${process.platform === "win32" ? ".exe" : ""}`,
    ),
    ["-test.run=^TestE2ERetainedCapacityFixture$", "-test.timeout=2m", "-test.v"],
    {
      cwd: path.resolve(process.cwd(), "../backend"),
      env: {
        ...process.env,
        KANDEV_E2E_CAPACITY_ROOT: fixtureRoot,
        KANDEV_E2E_CAPACITY_SESSION: sessionId,
        KANDEV_E2E_CAPACITY_ACTION: action,
      },
      timeout: 150_000,
      maxBuffer: 1 << 20,
    },
  ).catch((failure: unknown) => {
    const output = failure as { stdout?: string; stderr?: string };
    throw new Error(`Capacity fixture failed:\n${output.stdout ?? ""}\n${output.stderr ?? ""}`);
  });
  expect(result.stdout).toContain(
    action === "fill"
      ? "KANDEV_E2E_CAPACITY_FIXTURE:full_projected_backlog"
      : "KANDEV_E2E_CAPACITY_FIXTURE:projected_backlog_pruned",
  );
}
