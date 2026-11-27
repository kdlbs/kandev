import { test } from "../../fixtures/test-base";
import { freshStartSubmissionRecoveryScenario } from "../../helpers/fresh-start-submission-recovery";
import { useRegularMode } from "../../helpers/regular-mode";

test.describe("fresh-start submission recovery", () => {
  useRegularMode();

  test(
    "replays uploaded attachments once and retires the matching warning",
    freshStartSubmissionRecoveryScenario(false),
  );

  test(
    "replays an attachment-only submission after failed startup",
    freshStartSubmissionRecoveryScenario(false, true),
  );
});
