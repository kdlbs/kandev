import { randomUUID } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";

/** Hold a real mock-agent turn until the test has established its intermediate state. */
export function mockTurnGate(
  tmpDir: string,
  completeMessage: string,
  { announceStart = true }: { announceStart?: boolean } = {},
) {
  const marker = join(tmpDir, `mock-turn-${randomUUID()}.release`);
  return {
    prompt: [
      ...(announceStart ? ['e2e:message("Queue blocking turn started")'] : []),
      `e2e:await_file(${JSON.stringify(marker)})`,
      `e2e:message(${JSON.stringify(completeMessage)})`,
    ].join("\n"),
    release: () => writeFile(marker, "release"),
  };
}
