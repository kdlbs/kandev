import { test } from "../../fixtures/test-base";
import { exerciseMiniMaxSetup } from "./minimax-agent-helpers";

test("native MiniMax install, login and selected-model task work", async ({
  testPage,
  backend,
  apiClient,
  prCapture,
  seedData,
}, info) => {
  test.setTimeout(90_000);
  await exerciseMiniMaxSetup({
    page: testPage,
    backend,
    api: apiClient,
    info,
    capture: prCapture,
    seed: seedData,
  });
});
