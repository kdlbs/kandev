import fs from "node:fs";
import path from "node:path";
import type { GitHelper } from "../../helpers/git-helper";

const SAVED_HTML = "<!doctype html><html><body><p>Saved source</p></body></html>";
const SVG_ASSET =
  '<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><rect width="24" height="24" fill="#4ade80"/></svg>';
const HISTORY_SUFFIXES = [
  "01-continuity-history",
  "02-continuity-history",
  "03-continuity-history",
];

function previewPaths(suffix: string): string[] {
  return [
    `preview-${suffix}.html`,
    `preview-assets-${suffix}/preview.css`,
    `preview-assets-${suffix}/preview.js`,
    `preview-assets-${suffix}/logo.svg`,
  ];
}

function rollbackPreviewHistory(
  git: GitHelper,
  initialHead: string,
  attemptedPaths: string[],
  setupError: unknown,
): never {
  const errors = [setupError];
  try {
    git.exec(`git reset --hard ${initialHead}`);
  } catch (error) {
    errors.push(error);
  }
  for (const name of attemptedPaths) {
    try {
      git.deleteFile(name);
    } catch (error) {
      errors.push(error);
    }
  }
  if (errors.length > 1) {
    throw new AggregateError(errors, "Preview history rollback failed", { cause: setupError });
  }
  throw setupError;
}

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
  const directory = git.exec("git rev-parse --show-toplevel").trim();
  for (const suffix of HISTORY_SUFFIXES) {
    const assetDirectory = `preview-assets-${suffix}`;
    if (
      fs
        .lstatSync(path.join(directory, assetDirectory), { throwIfNoEntry: false })
        ?.isSymbolicLink()
    ) {
      throw new Error(`Preview history fixture directory is a symlink: ${assetDirectory}`);
    }
  }
  for (const name of HISTORY_SUFFIXES.flatMap(previewPaths)) {
    if (fs.lstatSync(path.join(directory, name), { throwIfNoEntry: false })) {
      throw new Error(`Preview history fixture path already exists: ${name}`);
    }
  }
  const attemptedPaths: string[] = [];
  const createPreviewFile = (name: string, content: string) => {
    attemptedPaths.push(name);
    git.createFile(name, content);
  };
  try {
    for (const suffix of HISTORY_SUFFIXES) {
      const fileName = `preview-${suffix}.html`;
      const assetDirectory = `preview-assets-${suffix}`;
      createPreviewFile(fileName, SAVED_HTML);
      createPreviewFile(`${assetDirectory}/preview.css`, "#css-status { font-weight: 700; }");
      createPreviewFile(`${assetDirectory}/preview.js`, nativePreviewScript(fileName));
      createPreviewFile(`${assetDirectory}/logo.svg`, SVG_ASSET);
      git.exec(
        `git add -- ${previewPaths(suffix)
          .map((name) => `"${name}"`)
          .join(" ")}`,
      );
      git.commit(`add ${fileName}`);
    }
  } catch (error) {
    rollbackPreviewHistory(git, initialHead, attemptedPaths, error);
  }
  return () => git.exec(`git reset --hard ${initialHead}`);
}
