import type { GitHelper } from "../../helpers/git-helper";

const SAVED_HTML = "<!doctype html><html><body><p>Saved source</p></body></html>";
const SVG_ASSET =
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="#4ade80"/></svg>';

function nativePreviewScript(entryName: string): string {
  return `(() => {
  const image = document.querySelector("#asset-image");
  const status = document.querySelector("#native-status");
  const value = document.querySelector("#value");
  const button = document.querySelector("#increment");
  const render = () => {
    status.textContent = [
      typeof ResizeObserver === "function" ? "api:available" : "api:missing",
      image.complete && image.naturalWidth > 0 ? "image:loaded" : "image:pending",
      location.pathname.endsWith("/${entryName}") ? "path:entry" : "path:wrong",
    ].join(" | ");
  };
  let count = 0;
  image.addEventListener("load", render);
  button.addEventListener("click", () => {
    count += 1;
    value.textContent = String(count);
  });
  render();
})();`;
}

export function seedContinuityPreviewHistory(git: GitHelper) {
  const initialHead = git.getCurrentSha();
  for (const suffix of [
    "01-continuity-history",
    "02-continuity-history",
    "03-continuity-history",
  ]) {
    const fileName = `preview-${suffix}.html`;
    const assetDirectory = `preview-assets-${suffix}`;
    git.createFile(fileName, SAVED_HTML);
    git.createFile(`${assetDirectory}/preview.css`, "#css-status { font-weight: 700; }");
    git.createFile(`${assetDirectory}/preview.js`, nativePreviewScript(fileName));
    git.createFile(`${assetDirectory}/logo.svg`, SVG_ASSET);
    git.stageAll();
    git.commit(`add ${fileName}`);
  }
  return () => git.exec(`git reset --hard ${initialHead}`);
}
