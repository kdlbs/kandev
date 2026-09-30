import { test } from "../../fixtures/test-base";
import { exerciseMiniMaxSetup } from "./minimax-agent-helpers";

test("native MiniMax setup and model profile work on phone", async ({
  testPage,
  backend,
  apiClient,
  prCapture,
}, info) => {
  test.setTimeout(90_000);
  await exerciseMiniMaxSetup(testPage, backend, apiClient, info, prCapture);
});
