import path from "node:path";
import { test, expect } from "../../fixtures/ssh-test-base";
import { startBastionAndTarget, stopBastionAndTarget } from "../../helpers/ssh-bastion";
import {
  createSSHRoute,
  createTestedSSHRoute,
  observeSSHRoute,
} from "../../helpers/ssh-managed-fixture";

test("a tested shared bastion reaches the target and cannot itself have a bastion", async ({
  backend,
}, testInfo) => {
  test.setTimeout(240_000);
  const handles = startBastionAndTarget(
    testInfo.workerIndex,
    path.join(backend.tmpDir, "ssh-bastion-shared"),
  );
  try {
    const entry = await createTestedSSHRoute(backend.baseUrl, "Entry bastion", handles.bastion);
    const direct = await createSSHRoute(backend.baseUrl, "No route from host", handles.target);
    await expect(observeSSHRoute(backend.baseUrl, direct)).rejects.toThrow();

    const target = await createTestedSSHRoute(
      backend.baseUrl,
      "Private target",
      handles.target,
      entry.destination.id,
    );
    expect(target.destination.verified_route).toBe(target.stamp);
    expect(target.bastion?.fingerprint).toBe(entry.destination.fingerprint);
    await expect(
      createSSHRoute(backend.baseUrl, "Invalid chain", handles.target, {
        bastionId: target.destination.id,
      }),
    ).rejects.toThrow("ssh_invalid_route");
  } finally {
    stopBastionAndTarget(handles);
  }
});
