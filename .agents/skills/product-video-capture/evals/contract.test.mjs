import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const evalDir = path.dirname(fileURLToPath(import.meta.url));
const skillDir = path.resolve(evalDir, "..");
const skill = fs.readFileSync(path.join(skillDir, "SKILL.md"), "utf8");
const bundle = [
  skill,
  ...fs
    .readdirSync(path.join(skillDir, "references"))
    .filter((name) => name.endsWith(".md"))
    .sort()
    .map((name) => fs.readFileSync(path.join(skillDir, "references", name), "utf8")),
].join("\n");
const routing = fs.readFileSync(
  path.resolve(skillDir, "..", "using-agent-skills", "SKILL.md"),
  "utf8",
);

test("defines exact source and delivery profiles", () => {
  assert.match(bundle, /3840x2400[^\n]{0,100}1920x1200/i);
  assert.match(bundle, /1290x2796/i);
  assert.match(bundle, /25 fps/i);
  assert.match(bundle, /(?:at most|maximum|max(?:imum)? zoom|cap(?:ped)?)\D{0,20}2\.0x/i);
});

test("keeps masters continuous, honest, and reversible", () => {
  assert.match(bundle, /one continuous take/i);
  assert.match(bundle, /no (?:internal )?cuts?[^\n]{0,80}(?:speed|speed ramps?)[^\n]{0,80}audio/i);
  assert.match(bundle, /reversible camera/i);
});

test("records dense semantic pointer and target geometry", () => {
  assert.match(bundle, /dense[^\n]{0,100}(?:pointer|cursor|touch)[^\n]{0,100}(?:samples|waypoints|metadata)/i);
  assert.match(bundle, /target (?:glyph )?bounds/i);
  assert.match(bundle, /glyph bounds/i);
  assert.match(bundle, /visibility interval/i);
});

test("uses center-biased widen-pan-tighten camera choreography", () => {
  assert.match(bundle, /center-biased/i);
  assert.match(bundle, /widen[^\n]{0,100}pan[^\n]{0,100}tighten/i);
  assert.match(bundle, /full (?:dialog|menu)[^\n]{0,80}(?:priority|visible|inside|frame)/i);
});

test("requires semantic evidence for the landing editorial profile", () => {
  assert.match(bundle, /cameraProfile[^\n]{0,40}landing-editorial/i);
  assert.match(bundle, /landing-editorial[^\n]{0,120}requires?[^\n]{0,80}focusTrack/i);
  assert.match(bundle, /landing-editorial[^\n]{0,120}requires?[^\n]{0,80}pointerTrack/i);
});

test("keeps semantic focus evidence separate from camera keyframes in landing examples", () => {
  const reference = fs.readFileSync(
    path.join(skillDir, "references", "camera-encoding.md"),
    "utf8",
  );
  const examples = reference.match(/```jsonc?[\s\S]*?```/g) ?? [];
  const landingExamples = examples.filter((example) =>
    example.includes('"cameraProfile": "landing-editorial"'),
  );

  assert.equal(landingExamples.length, 2);
  for (const example of landingExamples) {
    assert.match(example, /"focusTrack"\s*:/);
    assert.match(example, /"pointerTrack"\s*:/);
    assert.match(example, /"keyframes"\s*:/);
  }
});

test("names the settled penultimate loop frame in the acceptance gate", () => {
  assert.match(skill, /first, settled penultimate, and final/i);
  assert.doesNotMatch(skill, /first\/settled\/final/i);
});

test("explicitly rejects bad editorial shortcuts", () => {
  assert.match(bundle, /reject[^\n]{0,100}lazy global zoom/i);
  assert.match(bundle, /reject[^\n]{0,120}(?:zoom|camera)[^\n]{0,80}away from[^\n]{0,40}(?:active )?(?:cursor|pointer)/i);
  assert.match(bundle, /reject[^\n]{0,100}stale UI/i);
  assert.match(bundle, /reject[^\n]{0,120}wide shots?[^\n]{0,100}unreadable/i);
});

test("proves pointer containment and actual-size readability", () => {
  assert.match(bundle, /(?:cursor|pointer)[^\n]{0,100}(?:never leaves|leaving) (?:the )?frame/i);
  assert.match(bundle, /actual-size/i);
  assert.match(bundle, /contact sheet/i);
});

test("requires multi-format QA, hashes, provenance, and teardown", () => {
  for (const token of ["WebM", "MP4", "WebP", "SHA-256", "browser", "codec", "provenance", "teardown"]) {
    assert.match(bundle, new RegExp(token, "i"));
  }
});

test("always invokes seeding and records strictly from current origin/main", () => {
  assert.match(bundle, /always[^\n]{0,100}(?:invoke|run|use)[^\n]{0,80}product-demo-seeding/i);
  assert.match(routing, /product media always invokes[^\n]{0,100}product-demo-seeding/i);
  assert.match(bundle, /record only[^\n]{0,100}origin\/main/i);
  assert.match(bundle, /approved raw[^\n]{0,140}source SHA[^\n]{0,100}(?:still )?(?:equals|matches)[^\n]{0,80}origin\/main[^\n]{0,80}(?:otherwise|else)[^\n]{0,40}recapture/i);
  assert.doesNotMatch(bundle, /unless the user explicitly names another immutable revision/i);
  assert.doesNotMatch(bundle, /origin\/main[^\n]{0,40}by default/i);
});

test("proves dark theme without product DOM or CSS patching", () => {
  assert.match(bundle, /normal Kandev user setting[^\n]{0,100}prefers-color-scheme/i);
  assert.match(bundle, /never[^\n]{0,80}(?:patch the DOM|DOM patch)[^\n]{0,80}(?:inject CSS|CSS patch)/i);
  assert.match(bundle, /exact-profile rehearsal frame[^\n]{0,100}proves? the theme/i);
});

test("requires measured realtime recorder capacity and zero cadence loss", () => {
  assert.match(bundle, /measured[^\n]{0,100}realtime[^\n]{0,100}(?:recorder|encode|capacity)/i);
  assert.match(bundle, /zero[^\n]{0,60}(?:duplicated|duplicate|dup)[^\n]{0,60}(?:and|\/)[^\n]{0,60}(?:dropped|drop)[^\n]{0,60}frames/i);
  assert.match(bundle, /FFmpeg log/i);
});
