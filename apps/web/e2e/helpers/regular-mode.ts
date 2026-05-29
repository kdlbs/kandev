import { test } from "../fixtures/test-base";

/**
 * Run the current spec file's tests with the `office` feature DISABLED.
 *
 * The e2e profile (`profiles.yaml`) enables `office` for the whole suite, which
 * makes the sidebar "New Task" open the richer Office "New issue" dialog. Specs
 * that exercise the regular task-create flow (the `create-task-dialog` with
 * `task-title-input` / `submit-start-agent`) need the non-office experience, so
 * they call this at the top of their `describe`/module to restart the
 * worker-scoped backend with `KANDEV_FEATURES_OFFICE=false` and revert to the
 * profile default afterwards. `workers: 1` runs files sequentially, so the
 * override is cleanly scoped to this file.
 */
export function useRegularMode(): void {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_FEATURES_OFFICE: "false" });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });
}
