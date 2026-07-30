import { test } from "../../fixtures/test-base";
import { verifyStalledDialogCloseRecovery } from "./dialog-body-lock-helpers";

test("a stalled dialog close releases and can reacquire the body lock", async ({ testPage }) => {
  await verifyStalledDialogCloseRecovery(testPage, true);
});
