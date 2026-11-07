import fs from "node:fs";
import path from "node:path";
import zlib from "node:zlib";
import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

type Rgb = [number, number, number];

const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n;
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
  return c >>> 0;
});

function crc32(data: Buffer): number {
  let crc = 0xffffffff;
  for (const byte of data) crc = CRC_TABLE[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

function pngChunk(type: string, data: Buffer): Buffer {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body));
  return Buffer.concat([length, body, crc]);
}

/** Encodes an RGB image whose pixels come from `pixel`. */
export function encodePng(width: number, height: number, pixel: (x: number, y: number) => Rgb) {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header.set([8, 2, 0, 0, 0], 8);
  const rows: Buffer[] = [];
  for (let y = 0; y < height; y++) {
    const row = Buffer.alloc(1 + width * 3);
    for (let x = 0; x < width; x++) row.set(pixel(x, y), 1 + x * 3);
    rows.push(row);
  }
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    pngChunk("IHDR", header),
    pngChunk("IDAT", zlib.deflateSync(Buffer.concat(rows))),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

const BACKGROUND: Rgb = [240, 240, 240];
const HEADER: Rgb = [40, 40, 60];
const LABEL: Rgb = [90, 90, 90];
const CHANGE: Rgb = [200, 30, 30];

function within(x: number, y: number, box: [number, number, number, number]) {
  return x >= box[0] && x < box[0] + box[2] && y >= box[1] && y < box[1] + box[3];
}

function mockup(x: number, y: number): Rgb {
  if (y < 20) return HEADER;
  if (within(x, y, [20, 40, 40, 8])) return LABEL;
  return BACKGROUND;
}

/** The candidate differs from the baseline in exactly CHANGED_PIXELS pixels. */
export const CHANGED_PIXELS = 40;
export const REVIEW_IMAGE_SIZE = { width: 160, height: 100 };

export const reviewImages = {
  before: { id: "before", path: "review-before.png", alt: "Settings header before" },
  after: { id: "after", path: "review-after.png", alt: "Settings header after" },
  taller: { id: "taller", path: "review-taller.png", alt: "Settings header with taller page" },
  spacing: { id: "spacing", path: "review-spacing.png", alt: "Settings spacing" },
};

export function seedReviewImages(repositoryPath: string) {
  const { width, height } = REVIEW_IMAGE_SIZE;
  const files: Record<string, Buffer> = {
    [reviewImages.before.path]: encodePng(width, height, mockup),
    [reviewImages.after.path]: encodePng(width, height, (x, y) =>
      within(x, y, [20, 52, 10, 4]) ? CHANGE : mockup(x, y),
    ),
    [reviewImages.taller.path]: encodePng(width, height + 4, mockup),
    [reviewImages.spacing.path]: encodePng(120, 80, (x, y) => (y < 10 ? HEADER : BACKGROUND)),
  };
  for (const [name, bytes] of Object.entries(files)) {
    fs.writeFileSync(path.join(repositoryPath, name), bytes);
  }
}

const options = [
  { label: "Approve", description: "Accept" },
  { label: "Revise", description: "Request changes" },
];

export function reviewScript(firstImages = [reviewImages.before, reviewImages.after]) {
  const questions = [
    {
      id: "header",
      title: "Header",
      prompt: "Is the new header spacing acceptable?",
      images: firstImages,
      options,
    },
    {
      id: "spacing",
      title: "Spacing",
      prompt: "Does the settings spacing look right?",
      images: [reviewImages.spacing],
      options,
    },
  ];
  return `e2e:mcp:kandev:ask_user_question_kandev(${JSON.stringify({ questions })})\ne2e:message("Review questions complete.")`;
}

type Seed = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  agentProfileId: string;
  repositoryId: string;
};

export async function openReviewQuestions(page: Page, api: ApiClient, seed: Seed, script: string) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Review images",
    seed.agentProfileId,
    {
      description: script,
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Review fixture has no session");
  await waitForSessionState(api, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "Review question must block its source session",
    timeout: 90_000,
  });
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await expect(session.clarificationOverlay().getByTestId("question-review-open")).toBeVisible({
    timeout: 30_000,
  });
  return { task, session };
}

/** Records the image IDs requested through the authorized question-image route. */
export function recordImageRequests(page: Page) {
  const ids: string[] = [];
  page.on("request", (request) => {
    const match = /\/api\/v1\/clarification\/[^/]+\/images\/([^/?]+)/.exec(request.url());
    if (match) ids.push(decodeURIComponent(match[1]));
  });
  return ids;
}

export async function expectNoHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

/** The compact panel's first two thumbnails sit on one row. */
export async function expectThumbnailsShareRow(overlay: Locator) {
  const images = overlay.getByTestId("question-images-row").first().getByRole("img");
  await expect(images.nth(1)).toBeVisible();
  const first = (await images.nth(0).boundingBox())!;
  const second = (await images.nth(1).boundingBox())!;
  expect(Math.abs(first.y - second.y)).toBeLessThan(2);
  expect(second.x).toBeGreaterThan(first.x + first.width - 1);
}

/** Drags with real touch events from the center of `target` to `toX` on the same row. */
export async function touchDrag(page: Page, target: Locator, toX: number) {
  const box = (await target.boundingBox())!;
  const y = box.y + box.height / 2;
  const fromX = box.x + box.width / 2;
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Input.dispatchTouchEvent", {
    type: "touchStart",
    touchPoints: [{ x: fromX, y }],
  });
  for (let step = 1; step <= 10; step++) {
    const x = fromX + ((toX - fromX) * step) / 10;
    await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x, y }] });
  }
  await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
  await cdp.detach();
}
