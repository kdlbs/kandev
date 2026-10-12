import { test } from "../../fixtures/test-base";
import { freshStartSubmissionRecoveryScenario } from "../../helpers/fresh-start-submission-recovery";
import { useRegularMode } from "../../helpers/regular-mode";

test.describe("phone fresh-start submission recovery", () => {
  useRegularMode();

  test(
    "replays uploaded attachments once and restores the phone composer",
    freshStartSubmissionRecoveryScenario(true),
  );
});
