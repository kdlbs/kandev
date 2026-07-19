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
const qaChecklist = fs.readFileSync(
  path.join(skillDir, "references", "qa-checklist.md"),
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

test("keeps the DOM overlay and real browser pointer in lockstep", () => {
  assert.match(bundle, /DOM overlay and (?:the )?real browser pointer/i);
  assert.match(bundle, /at every sample/i);
  assert.match(bundle, /never animate it independently[\s\S]{0,260}move the browser pointer/i);
  assert.match(bundle, /zero endpoint displacement/i);
  assert.match(bundle, /normal playback speed/i);
});

test("keeps target and pointer glyph geometry distinct over the full movement", () => {
  assert.match(
    bundle,
    /target glyph bounds[^\n]{0,140}(?:rendered|visual)[^\n]{0,80}(?:content|glyph)[^\n]{0,80}(?:inside|within)[^\n]{0,40}(?:target|control)/i,
  );
  assert.match(
    bundle,
    /never[^\n]{0,120}(?:reuse|alias|copy)[^\n]{0,100}(?:pointer|cursor)[^\n]{0,80}target glyph bounds/i,
  );
  assert.match(
    bundle,
    /visibility interval[^\n]{0,120}(?:begins|starts)[^\n]{0,60}motion start[^\n]{0,80}(?:not|rather than)[^\n]{0,40}arrival/i,
  );
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

test("rejects camera breathing and abrupt semantic travel", () => {
  assert.match(bundle, /reject[^\n]{0,120}(?:zoom )?breathing/i);
  assert.match(bundle, /one smooth[^\n]{0,80}(?:establish|tighten)/i);
  assert.match(bundle, /semantic (?:camera )?(?:move|travel)[^\n]{0,100}(?:at least|minimum)[^\n]{0,30}1\.2 seconds/i);
  assert.match(bundle, /readable hold[^\n]{0,100}(?:0\.9[^\n]{0,20}1\.5 seconds|900[^\n]{0,20}1,?500 ?ms)/i);
  assert.match(bundle, /routine pan[^\n]{0,120}(?:median|p95)[^\n]{0,80}0\.11/i);
});

test("keeps the camera timing exceptions explicit in the QA checklist", () => {
  assert.match(
    qaChecklist,
    /1\.2 seconds[^\n]{0,120}(?:except|exception)[^\n]{0,100}(?:already-settled|already settled)[^\n]{0,80}focus region/i,
  );
  assert.match(
    qaChecklist,
    /single pan peak[^\n]{0,100}(?:higher|exceed)[^\n]{0,100}declared long journey[^\n]{0,100}profile cap/i,
  );
});

test("audits readability at the production landing theater size", () => {
  assert.match(bundle, /964[^\n]{0,20}602/i);
  assert.match(bundle, /actual (?:landing )?(?:player|theater|stage)/i);
  assert.match(bundle, /meaningful glyph[^\n]{0,100}(?:at least|minimum)[^\n]{0,20}9 ?px/i);
  assert.match(bundle, /reject[^\n]{0,140}(?:artifact|compression)[^\n]{0,100}(?:text|glyph|label)/i);
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
  assert.match(bundle, /time_base/i);
  assert.match(bundle, /(?:best_effort_timestamp|pkt_dts|pkt_pts)(?!_time)/i);
  assert.match(bundle, /integer[^\n]{0,100}(?:timestamp|tick)/i);
});
