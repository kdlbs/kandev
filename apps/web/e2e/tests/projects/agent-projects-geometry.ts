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
  const controls = form.locator("input:not([type=checkbox]), select, footer button");
  for (const control of await controls.all()) {
    const box = await control.boundingBox();
    expect(box).not.toBeNull();
    if (mobile) expect.soft(box!.height).toBeGreaterThanOrEqual(44);
    else expect.soft(Math.abs(box!.height - 28)).toBeLessThanOrEqual(1);
  }
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
}
