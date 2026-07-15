import { test, expect } from "../../fixtures/test-base";

/**
 * Telemetry consent surface.
 *
 * E2E backends run with KANDEV_E2E_MOCK, which hard-disables telemetry
 * (internal/telemetry.EnvDisabled) — the strongest guarantee that CI can
 * never emit real events. That means the enabled-path UI toggle cannot be
 * exercised here by design; grant/deny mechanics are covered at the API
 * layer below and exhaustively in backend unit tests
 * (internal/telemetry/*_test.go). This spec pins the wiring: the settings
 * page renders, the env-disabled state is honest in both API and UI, and
 * the consent API round-trips a preference without ever enabling emission.
 *
 * Single ordered test on purpose: the backend (and its consent row) is
 * worker-scoped, so splitting grant/deny assertions into separate tests
 * would create order-dependent flakes.
 */
test.describe("Telemetry settings", () => {
  test("page renders env-disabled state and consent API round-trips", async ({
    testPage,
    backend,
  }) => {
    // 1. Fresh e2e install: consent unasked, hard-disabled by environment.
    const initial = await (await fetch(`${backend.baseUrl}/api/v1/telemetry/consent`)).json();
    expect(initial.status).toBe("unasked");
    expect(initial.env_disabled).toBe(true);

    // 2. Settings page renders with the banner and a disabled switch.
    await testPage.goto("/settings/system/telemetry");
    await expect(testPage.getByTestId("system-page-title")).toHaveText("Telemetry");
    await expect(testPage.getByTestId("telemetry-settings")).toBeVisible();
    await expect(testPage.getByText("Disabled by environment")).toBeVisible();
    await expect(testPage.getByTestId("telemetry-consent-switch")).toBeDisabled();

    // 3. The consent API still records the preference (grant mints an
    //    install id, deny clears it) — emission stays impossible in e2e.
    const grantRes = await fetch(`${backend.baseUrl}/api/v1/telemetry/consent`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "granted" }),
    });
    expect(grantRes.status).toBe(200);
    const granted = await grantRes.json();
    expect(granted.status).toBe("granted");
    expect(granted.install_id).toBeTruthy();
    expect(granted.env_disabled).toBe(true);

    const denyRes = await fetch(`${backend.baseUrl}/api/v1/telemetry/consent`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "denied" }),
    });
    expect(denyRes.status).toBe(200);
    const denied = await denyRes.json();
    expect(denied.status).toBe("denied");
    expect(denied.install_id).toBeFalsy();

    // 4. Invalid statuses are rejected.
    const badRes = await fetch(`${backend.baseUrl}/api/v1/telemetry/consent`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: "unasked" }),
    });
    expect(badRes.status).toBe(400);
  });
});
