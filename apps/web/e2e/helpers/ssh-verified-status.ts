import { writeFileSync } from "node:fs";
import { expect, test, type Locator, type Page } from "@playwright/test";
import type { SSHRoute } from "../../lib/types/http-ssh-connections";
import { seedSSHLayout } from "./ssh-management-layout";

async function badgeColors(badge: Locator) {
  return badge.evaluate((element) => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 1;
    const context = canvas.getContext("2d")!;
    context.fillStyle = "white";
    context.fillRect(0, 0, 1, 1);
    const ancestors: Element[] = [];
    for (let node: Element | null = element; node; node = node.parentElement)
      ancestors.unshift(node);
    for (const node of ancestors) {
      context.fillStyle = getComputedStyle(node).backgroundColor;
      context.fillRect(0, 0, 1, 1);
    }
    const background = [...context.getImageData(0, 0, 1, 1).data];
    context.fillStyle = getComputedStyle(element).color;
    context.fillRect(0, 0, 1, 1);
    const foreground = [...context.getImageData(0, 0, 1, 1).data];
    const luminance = (channels: number[]) =>
      channels.slice(0, 3).reduce((sum, value, index) => {
        const channel = value / 255;
        return (
          sum +
          (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4) *
            [0.2126, 0.7152, 0.0722][index]
        );
      }, 0);
    const values = [luminance(background), luminance(foreground)].sort((a, b) => b - a);
    return {
      green: foreground[1] > foreground[0] + 10 && foreground[1] > foreground[2] + 10,
      contrast: (values[0] + 0.05) / (values[1] + 0.05),
    };
  });
}

function statusFixtures(route: SSHRoute): SSHRoute[] {
  const ready: SSHRoute = {
    ...route,
    destination: {
      ...route.destination,
      id: "status-ready",
      name: "Verified fixture",
      fingerprint: "public-pin",
      verified_route: route.stamp,
    },
  };
  const routes = [ready];
  for (const state of [
    "stale",
    "unchecked",
    "incomplete",
    "unavailable",
    "untrusted",
    "uncertain",
  ]) {
    const next = structuredClone(ready);
    next.destination.id = `status-${state}`;
    next.destination.name = `Status ${state}`;
    if (state === "stale") next.stamp += "-changed";
    if (state === "unchecked") next.destination.verified_route = "";
    if (state === "incomplete") next.destination.user = "";
    if (state === "unavailable") next.destination.identity_id = "missing";
    if (state === "untrusted") next.destination.fingerprint = "";
    if (state === "uncertain")
      next.latest_enrollment = {
        request_id: "uncertain",
        connection_id: next.destination.id,
        key_fingerprint: "public-key",
        stamp: next.stamp,
        created_at: "2026-10-28T00:00:00Z",
        state: "outcome_unknown",
      };
    routes.push(next);
  }
  return routes;
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.14
// UI fixtures prove presentation and fresh-failure handling, not SSH authentication.
export async function exerciseSSHVerifiedStatus(page: Page, baseUrl: string) {
  const { route } = await seedSSHLayout(baseUrl);
  const routes = statusFixtures(route);
  await page.route("**/api/v1/ssh/managed/connections", (request) =>
    request.fulfill({ json: routes }),
  );
  await page.route("**/api/v1/ssh/managed/connections/status-ready/test", (request) =>
    request.fulfill({ status: 400, json: { error: "ssh_auth_failed" } }),
  );
  await page.goto("/settings/executors/ssh-connections");
  const measurements: Array<{ theme: string; location: string; contrast: number }> = [];
  for (const theme of ["light", "dark"]) {
    await page.evaluate((value) => localStorage.setItem("theme", value), theme);
    await page.reload();
    if (theme === "dark") await expect(page.locator("html")).toHaveClass(/dark/);
    else await expect(page.locator("html")).not.toHaveClass(/dark/);
    for (const [index, fixture] of routes.entries()) {
      const badge = page.locator(`a[id="ssh-row-${fixture.destination.id}"] [data-slot="badge"]`);
      await expect(badge).toBeVisible();
      const colors = await badgeColors(badge);
      expect(colors.green, fixture.destination.name).toBe(index === 0);
      if (index === 0) {
        expect(colors.contrast).toBeGreaterThanOrEqual(4.5);
        measurements.push({ theme, location: "collection", contrast: colors.contrast });
      }
    }
    await page.screenshot({
      path: test.info().outputPath(`ssh-collection-${theme}.png`),
      fullPage: true,
    });
    await page.locator('a[id="ssh-row-status-ready"]').click();
    const badge = page.locator('[data-slot="badge"]').filter({ hasText: "SSH verified" });
    const colors = await badgeColors(badge);
    expect(colors.green).toBe(true);
    expect(colors.contrast).toBeGreaterThanOrEqual(4.5);
    measurements.push({ theme, location: "detail", contrast: colors.contrast });
    await page.getByRole("button", { name: "Back to connections", exact: true }).click();
  }
  const contrastPath = test.info().outputPath("ssh-status-contrast.json");
  writeFileSync(contrastPath, JSON.stringify(measurements, null, 2));
  await test
    .info()
    .attach("ssh-status-contrast", { path: contrastPath, contentType: "application/json" });
  await page.locator('a[id="ssh-row-status-ready"]').click();
  const tested = page.waitForResponse((response) => response.url().endsWith("/status-ready/test"));
  await page.getByRole("button", { name: "Test SSH access", exact: true }).click();
  expect((await tested).status()).toBe(400);
  const failed = page.locator('[data-slot="badge"]').filter({ hasText: "Needs attention" });
  await expect(failed).toBeVisible();
  // The badge color transition settles after its state label changes.
  await expect.poll(async () => (await badgeColors(failed)).green).toBe(false);
  await page.getByRole("button", { name: "Back to connections", exact: true }).click();
  const rowBadge = page.locator('a[id="ssh-row-status-ready"] [data-slot="badge"]');
  await expect(rowBadge).toHaveText("Needs attention");
  await expect.poll(async () => (await badgeColors(rowBadge)).green).toBe(false);
}
