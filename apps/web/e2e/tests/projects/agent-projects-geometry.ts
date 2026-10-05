import { expect, type Locator } from "@playwright/test";

export async function settleProjectSurface(surface: Locator) {
  await surface.evaluate(async (element) => {
    await Promise.all(
      element
        .getAnimations({ subtree: true })
        .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity)
        .map((animation) => animation.finished.catch(() => undefined)),
    );
  });
}

export async function assertProjectFormGeometry(form: Locator, mobile: boolean) {
  await settleProjectSurface(form);
  await assertStandardControls(form, mobile);
  await assertTextareaGeometry(form, mobile);
  await assertCompactTargets(form, mobile);
  await expect(form.getByTestId("agent-project-project-help")).toBeVisible();
  if (!mobile) {
    const [heading, name] = await Promise.all([
      form.getByRole("heading").boundingBox(),
      form.getByTestId("agent-project-name").boundingBox(),
    ]);
    expect.soft(name!.x).toBeCloseTo(heading!.x, 0);
  } else {
    await assertFullHeightProjectSurface(form);
  }
}

export async function assertProjectWorkerProfilesGeometry(form: Locator, mobile: boolean) {
  const advanced = form.getByTestId("agent-project-advanced");
  if (!(await advanced.evaluate((element) => element.hasAttribute("open")))) {
    await advanced.locator("summary").click();
  }
  const economy = form.getByTestId("agent-project-economy-profile-select");
  const frontier = form.getByTestId("agent-project-frontier-profile-select");
  await expect(economy).toBeVisible();
  await expect(frontier).toBeVisible();
  const [economyBox, frontierBox] = await Promise.all([
    economy.boundingBox(),
    frontier.boundingBox(),
  ]);
  expect(economyBox).not.toBeNull();
  expect(frontierBox).not.toBeNull();
  if (mobile) {
    expect.soft(Math.abs(economyBox!.x - frontierBox!.x)).toBeLessThanOrEqual(2);
    expect.soft(frontierBox!.y).toBeGreaterThan(economyBox!.y);
  } else {
    expect.soft(Math.abs(economyBox!.y - frontierBox!.y)).toBeLessThanOrEqual(2);
    expect.soft(frontierBox!.x).toBeGreaterThan(economyBox!.x);
  }
}

async function assertStandardControls(form: Locator, mobile: boolean) {
  const controls = form.locator("input:not([type=checkbox]), select, footer button");
  for (const control of await controls.all()) {
    if (!(await control.isVisible())) continue;
    const box = await control.boundingBox();
    expect(box).not.toBeNull();
    if (mobile) {
      expect.soft(box!.height).toBeGreaterThanOrEqual(44);
      expect.soft(box!.width).toBeGreaterThanOrEqual(44);
    } else expect.soft(Math.abs(box!.height - 28)).toBeLessThanOrEqual(1);
  }
}

async function assertTextareaGeometry(form: Locator, mobile: boolean) {
  for (const textarea of await form.locator("textarea").all()) {
    if (!(await textarea.isVisible())) continue;
    const box = await textarea.boundingBox();
    expect(box).not.toBeNull();
    if (mobile) {
      expect.soft(box!.height).toBeGreaterThanOrEqual(44);
      expect.soft(box!.width).toBeGreaterThanOrEqual(44);
    } else {
      expect.soft(box!.height).toBeGreaterThanOrEqual(128);
    }
  }
}

async function assertCompactTargets(form: Locator, mobile: boolean) {
  const compactTargets = form.locator(
    '[data-testid="agent-project-add-repository"], [data-testid="agent-project-remove-repository"], [data-testid$="-help"], [data-testid="agent-project-advanced"] > summary',
  );
  for (const target of await compactTargets.all()) {
    if (!(await target.isVisible())) continue;
    const box = await target.boundingBox();
    expect(box).not.toBeNull();
    if (mobile) {
      expect.soft(box!.height).toBeGreaterThanOrEqual(44);
      expect.soft(box!.width).toBeGreaterThanOrEqual(44);
    } else {
      expect.soft(Math.abs(box!.height - 28)).toBeLessThanOrEqual(1);
      if ((await target.getAttribute("data-testid"))?.endsWith("-help")) {
        expect.soft(Math.abs(box!.width - 28)).toBeLessThanOrEqual(1);
      }
    }
  }
}

export async function assertProjectRemotePickerGeometry(
  form: Locator,
  mobile: boolean,
  screenshotPath?: string,
) {
  const page = form.page();
  const addRepository = form.getByTestId("agent-project-add-repository");
  await addRepository.click();
  const pickerSurface = page.locator("[data-radix-popper-content-wrapper]:visible").last();
  const visibleControls = page.locator(
    '[data-testid="remote-repo-input"], [data-testid="remote-repo-option"]:visible, [data-testid="agent-project-existing-repository-option"]:visible, [data-testid="remote-repo-provider-tabs"] button:visible',
  );
  await expect(page.getByTestId("remote-repo-input")).toBeVisible();
  await settleProjectSurface(pickerSurface);
  if (screenshotPath) await page.screenshot({ path: screenshotPath });
  for (const control of await visibleControls.all()) {
    if (!(await control.isVisible())) continue;
    const box = await control.boundingBox();
    expect(box).not.toBeNull();
    if (mobile) {
      expect.soft(box!.height).toBeGreaterThanOrEqual(44);
      expect.soft(box!.width).toBeGreaterThanOrEqual(44);
    } else {
      expect.soft(Math.abs(box!.height - 28)).toBeLessThanOrEqual(1);
    }
  }
  await page.keyboard.press("Escape");
}

export async function assertFullHeightProjectSurface(surface: Locator) {
  await settleProjectSurface(surface);
  const geometry = await surface.evaluate((element) => {
    const box = element.getBoundingClientRect();
    return { top: box.top, height: box.height, viewportHeight: window.innerHeight };
  });
  expect.soft(Math.abs(geometry.top)).toBeLessThanOrEqual(1);
  expect.soft(Math.abs(geometry.height - geometry.viewportHeight)).toBeLessThanOrEqual(1);
}

export async function assertProjectTouchTarget(control: Locator) {
  const box = await control.boundingBox();
  expect(box).not.toBeNull();
  expect.soft(box!.height).toBeGreaterThanOrEqual(44);
  expect.soft(box!.width).toBeGreaterThanOrEqual(44);
}
