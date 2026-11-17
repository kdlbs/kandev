import { expect, type Locator } from "@playwright/test";

type GlyphSize = 14 | 16;

export async function expectConflictTriangleSize(icon: Locator) {
  const triangle = await icon.getByTestId("pr-merge-conflict-warning").locator("svg").boundingBox();
  expect(triangle).not.toBeNull();
  if (!triangle) return;
  expect(triangle.width).toBeCloseTo(8, 1);
  expect(triangle.height).toBeCloseTo(8, 1);
}

export async function expectConflictCornerGeometry(icon: Locator, size: GlyphSize) {
  const geometry = await icon.evaluate((element) => {
    const conflict = element.querySelector<HTMLElement>(
      '[data-testid="pr-merge-conflict-warning"]',
    );
    const baseGlyph = conflict?.parentElement;
    const pullRequest = baseGlyph?.querySelector<SVGElement>(".tabler-icon-git-pull-request");
    const allocatedGlyph = baseGlyph?.parentElement;
    if (!baseGlyph || !pullRequest || !allocatedGlyph || !conflict) return null;
    const glyphBox = baseGlyph.getBoundingClientRect();
    const allocatedBox = allocatedGlyph.getBoundingClientRect();
    const pullRequestBox = pullRequest.getBoundingClientRect();
    const conflictBox = conflict.getBoundingClientRect();
    return {
      glyph: { width: glyphBox.width, height: glyphBox.height },
      allocated: { width: allocatedBox.width },
      pullRequest: { right: pullRequestBox.right, top: pullRequestBox.top },
      conflict: {
        left: conflictBox.left,
        top: conflictBox.top,
        width: conflictBox.width,
        height: conflictBox.height,
      },
    };
  });

  expect(geometry).not.toBeNull();
  if (!geometry) return;
  expect(geometry.glyph.width).toBeCloseTo(size, 1);
  expect(geometry.glyph.height).toBeCloseTo(size, 1);
  expect(geometry.allocated.width).toBeCloseTo(geometry.glyph.width, 1);
  expect(geometry.conflict.width).toBeCloseTo(10, 1);
  expect(geometry.conflict.height).toBeCloseTo(10, 1);
  expect(geometry.conflict.left).toBeLessThan(geometry.pullRequest.right);
  expect(geometry.conflict.top).toBeLessThan(geometry.pullRequest.top);
  await expectConflictTriangleSize(icon);
}
