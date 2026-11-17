"use strict";

const fs = require("node:fs");
const { GitHubClient, normalizeRepoPath } = require("./pr-docs.cjs");

const STATUS_CONTEXT = "UI screenshots";
const EXCEPTION_LABEL = "no-screenshots-allow";
const MAX_EVALUATION_ATTEMPTS = 3;
const MAX_CHANGED_FILES = 3000;
const MARKDOWN_ESCAPE_CHARACTERS = new Set([
  "\\",
  "`",
  "*",
  "_",
  "{",
  "}",
  "[",
  "]",
  "(",
  ")",
  "#",
  "+",
  "-",
  ".",
  "!",
  "|",
]);
const UI_ROOTS = ["apps/web/", "apps/packages/ui/", "apps/desktop/src/"];
const UI_EXTENSIONS = new Set([
  ".apng",
  ".avif",
  ".cjs",
  ".css",
  ".gif",
  ".html",
  ".ico",
  ".jpeg",
  ".jpg",
  ".js",
  ".jsx",
  ".json",
  ".less",
  ".mjs",
  ".mov",
  ".mp4",
  ".otf",
  ".png",
  ".sass",
  ".scss",
  ".svg",
  ".svelte",
  ".ttf",
  ".ts",
  ".tsx",
  ".vue",
  ".webm",
  ".webp",
  ".woff",
  ".woff2",
]);
const TOOL_CONFIG_NAMES = new Set([
  "components.json",
  "eslint.config.js",
  "eslint.config.mjs",
  ".eslintrc.cjs",
  ".eslintrc.js",
  "jest.config.js",
  "package.json",
  "playwright.config.ts",
  "postcss.config.js",
  ".prettierrc.cjs",
  ".prettierrc.js",
  "tailwind.config.js",
  "tsconfig.json",
  "vite.config.js",
  "vite.config.ts",
  "vitest.config.ts",
]);
const STATUS_DESCRIPTIONS = {
  no_ui_changes: "No UI files changed",
  screenshots_present: "Screenshot evidence present",
  maintainer_exception: "Maintainer screenshot exception",
  screenshots_missing: "Screenshot evidence missing",
  error: "Screenshot evaluation error",
};

function markdownSafe(value) {
  return String(value ?? "")
    .replace(/[\u0000-\u001f\u007f-\u009f]/g, " ")
    .replace(/[<>]/g, "")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 500);
}

function markdownEscaped(value) {
  return [...markdownSafe(value)]
    .map((character) =>
      MARKDOWN_ESCAPE_CHARACTERS.has(character) ? `\\${character}` : character,
    )
    .join("");
}

function getChangedPaths(changedFiles) {
  if (!Array.isArray(changedFiles)) {
    throw new Error("GitHub changed-file response is not a list");
  }
  const paths = [];
  const seen = new Set();
  for (const entry of changedFiles) {
    const candidates =
      typeof entry === "string"
        ? [entry]
        : entry?.status === "renamed"
          ? [entry.filename, entry.previous_filename]
          : [entry?.filename];
    if (typeof candidates[0] !== "string" || candidates[0].length === 0) {
      throw new Error("GitHub changed-file entry has no filename");
    }
    for (const candidate of candidates) {
      if (candidate == null) {
        continue;
      }
      const pathname = normalizeRepoPath(candidate);
      if (!seen.has(pathname)) {
        seen.add(pathname);
        paths.push(pathname);
      }
    }
  }
  return paths;
}

function isUICandidatePath(pathname) {
  return (
    pathname === "apps/desktop/index.html" ||
    UI_ROOTS.some((root) => pathname.startsWith(root))
  );
}

function exclusionReason(pathname) {
  const basename = pathname.slice(pathname.lastIndexOf("/") + 1).toLowerCase();
  if (
    /(?:^|\/)(?:docs|e2e|scripts|test|tests|__tests__|__fixtures__|storybook|\.storybook)(?:\/|$)/i.test(
      pathname,
    )
  ) {
    return "non-runtime directory";
  }
  if (
    /\.(?:test|spec)\.[^.]+$/i.test(basename) ||
    /\.stories\.[^.]+$/i.test(basename)
  ) {
    return "test or story file";
  }
  if (
    ["pnpm-lock.yaml", "package-lock.json", "yarn.lock", "bun.lockb"].includes(
      basename,
    )
  ) {
    return "dependency lock file";
  }
  if (
    /^apps\/web\/src\/locales\/[^/]+\/(?:[^/]+\/)*[^/]+\.json$/i.test(
      pathname,
    ) ||
    /\.(?:po|pot)$/i.test(basename)
  ) {
    return "translation catalog";
  }
  if (
    TOOL_CONFIG_NAMES.has(basename) ||
    /^(?:vite|vitest|jest|eslint|oxlint|postcss|tailwind|playwright)\.config\.[^.]+$/i.test(
      basename,
    ) ||
    /^tsconfig(?:\.[^.]+)?\.json$/i.test(basename) ||
    /^eslint\.[^.]+\.(?:js|mjs|cjs|ts)$/i.test(basename) ||
    /^\.prettierrc\.[^.]+$/i.test(basename)
  ) {
    return "tool configuration";
  }
  const extension = basename.includes(".")
    ? `.${basename.split(".").pop()}`
    : "";
  if (!UI_EXTENSIONS.has(extension)) {
    return "not a runtime source or visual asset";
  }
  return undefined;
}

function classifyUIChanges(changedFiles) {
  const paths = getChangedPaths(changedFiles);
  const candidatePaths = [];
  const excludedPaths = [];
  for (const pathname of paths) {
    if (!isUICandidatePath(pathname)) {
      continue;
    }
    if (exclusionReason(pathname)) {
      excludedPaths.push(pathname);
    } else {
      candidatePaths.push(pathname);
    }
  }
  return {
    candidatePaths,
    excludedPaths,
    requiresEvidence: candidatePaths.length > 0,
  };
}

function removeInlineCodeSpans(markdown) {
  const characters = markdown.split("");
  let cursor = 0;
  while (cursor < markdown.length) {
    const opener = markdown.indexOf("`", cursor);
    if (opener < 0) {
      break;
    }
    let openerBackslashes = 0;
    for (
      let index = opener - 1;
      index >= 0 && markdown[index] === "\\";
      index--
    ) {
      openerBackslashes++;
    }
    let openerEnd = opener + 1;
    while (markdown[openerEnd] === "`") {
      openerEnd++;
    }
    const delimiterLength = openerEnd - opener;
    if (openerBackslashes % 2 === 1) {
      cursor = openerEnd;
      continue;
    }

    let closer = openerEnd;
    while (closer < markdown.length) {
      closer = markdown.indexOf("`", closer);
      if (closer < 0) {
        break;
      }
      let closerEnd = closer + 1;
      while (markdown[closerEnd] === "`") {
        closerEnd++;
      }
      if (closerEnd - closer === delimiterLength) {
        for (let index = opener; index < closerEnd; index++) {
          if (characters[index] !== "\n") {
            characters[index] = " ";
          }
        }
        cursor = closerEnd;
        break;
      }
      closer = closerEnd;
    }
    if (closer < 0) {
      cursor = openerEnd;
    }
  }
  return characters.join("");
}

function withoutMarkdownNoise(body) {
  const withoutComments = body.replace(/<!--[\s\S]*?-->/g, "");
  const output = [];
  let fence;
  for (const line of withoutComments.split(/\r?\n/)) {
    const match = /^ {0,3}(`{3,}|~{3,})/.exec(line);
    if (!fence && match) {
      fence = { char: match[1][0], length: match[1].length };
      output.push("");
      continue;
    }
    if (fence) {
      const close = new RegExp(`^ {0,3}${fence.char}{${fence.length},}\\s*$`);
      if (close.test(line)) {
        fence = undefined;
      }
      output.push("");
      continue;
    }
    if (/^(?: {4}|\t)/.test(line)) {
      output.push("");
      continue;
    }
    output.push(line);
  }
  return removeInlineCodeSpans(output.join("\n"));
}

function screenshotSection(body) {
  if (typeof body !== "string") {
    throw new Error("pull request body is missing or invalid");
  }
  const lines = withoutMarkdownNoise(body).split("\n");
  const start = lines.findIndex((line) =>
    /^##[ \t]+Screenshots[ \t]*#*[ \t]*$/i.test(line),
  );
  if (start < 0) {
    return "";
  }
  const end = lines.findIndex(
    (line, index) => index > start && /^##[ \t]+/.test(line),
  );
  return lines.slice(start + 1, end < 0 ? lines.length : end).join("\n");
}

function hasScreenshotEmbed(body) {
  const section = screenshotSection(body);
  const embeds = section.matchAll(
    /!\[([^\]\n]*)\]\(\s*(<[^>\s]+>|[^\s)]+)(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\s*\)/g,
  );
  for (const embed of embeds) {
    const trailingBackslashes =
      /\\+$/.exec(section.slice(0, embed.index))?.[0].length ?? 0;
    if (trailingBackslashes % 2 === 1) {
      continue;
    }
    const alt = embed[1].trim();
    const target =
      embed[2].startsWith("<") && embed[2].endsWith(">")
        ? embed[2].slice(1, -1)
        : embed[2];
    if (!alt) {
      continue;
    }
    try {
      const url = new URL(target);
      if (
        url.protocol === "https:" &&
        url.hostname &&
        !url.username &&
        !url.password
      ) {
        return true;
      }
    } catch {
      // Invalid destinations do not provide screenshot evidence.
    }
  }
  return false;
}

function validatePullRequest(pullRequest, number) {
  if (!pullRequest || pullRequest.number !== number) {
    throw new Error("GitHub returned incomplete pull request metadata");
  }
  const headSha = pullRequest.head?.sha;
  const baseSha = pullRequest.base?.sha;
  if (
    !/^[a-f0-9]{40,64}$/i.test(headSha ?? "") ||
    !/^[a-f0-9]{40,64}$/i.test(baseSha ?? "")
  ) {
    throw new Error("pull request has incomplete revision metadata");
  }
  if (
    !Number.isInteger(pullRequest.changed_files) ||
    pullRequest.changed_files < 0 ||
    pullRequest.changed_files > MAX_CHANGED_FILES
  ) {
    throw new Error("pull request has an invalid changed-file count");
  }
  if (
    !Array.isArray(pullRequest.labels) ||
    typeof pullRequest.base?.ref !== "string"
  ) {
    throw new Error("pull request has incomplete label or base metadata");
  }
  if (
    pullRequest.labels.some((label) => !label || typeof label.name !== "string")
  ) {
    throw new Error("pull request has incomplete label metadata");
  }
  if (pullRequest.body != null && typeof pullRequest.body !== "string") {
    throw new Error("pull request body is invalid");
  }
  return pullRequest;
}

function metadataSnapshot(pullRequest) {
  return {
    baseRef: pullRequest.base.ref,
    baseSha: pullRequest.base.sha,
    body: pullRequest.body ?? "",
    changedFiles: pullRequest.changed_files,
    headSha: pullRequest.head.sha,
    labels: pullRequest.labels
      .map((label) => label?.name)
      .filter((name) => typeof name === "string")
      .sort(),
  };
}

function sameMetadata(left, right) {
  return (
    JSON.stringify(metadataSnapshot(left)) ===
    JSON.stringify(metadataSnapshot(right))
  );
}

async function evaluateSnapshot({ client, pullRequest, pullNumber }) {
  const changedFiles = await client.listFiles(
    pullNumber,
    pullRequest.changed_files,
  );
  if (
    !Array.isArray(changedFiles) ||
    changedFiles.length !== pullRequest.changed_files
  ) {
    throw new Error("GitHub changed-file response is incomplete");
  }
  const classification = classifyUIChanges(changedFiles);
  let status;
  let message;
  if (!classification.requiresEvidence) {
    status = "no_ui_changes";
    message = "No UI candidate files changed.";
  } else if (
    pullRequest.labels.some((label) => label?.name === EXCEPTION_LABEL)
  ) {
    status = "maintainer_exception";
    message = `Maintainer exception applied with ${EXCEPTION_LABEL}.`;
  } else if (hasScreenshotEmbed(pullRequest.body ?? "")) {
    status = "screenshots_present";
    message = "An HTTPS Markdown image is present in the Screenshots section.";
  } else {
    status = "screenshots_missing";
    message =
      "Add an HTTPS Markdown image with descriptive alt text under ## Screenshots, or ask a maintainer to apply no-screenshots-allow.";
  }
  return {
    candidateCount: classification.candidatePaths.length,
    candidatePaths: classification.candidatePaths,
    excludedCount: classification.excludedPaths.length,
    headSha: pullRequest.head.sha,
    message,
    pullNumber,
    status,
  };
}

function errorResult(message, pullNumber, headSha) {
  return {
    candidateCount: 0,
    candidatePaths: [],
    excludedCount: 0,
    headSha,
    message:
      markdownSafe(message) ||
      "Required GitHub metadata could not be read or validated.",
    pullNumber,
    status: "error",
  };
}

function statusState(result) {
  if (result.status === "screenshots_missing") {
    return "failure";
  }
  if (result.status === "error") {
    return "error";
  }
  return "success";
}

function resultSummary(result) {
  const lines = [
    "## UI screenshot evidence",
    "",
    `Pull request: #${result.pullNumber ?? "unknown"}`,
    `Result: **${result.status}**`,
    `UI candidate files: ${result.candidateCount}`,
    `Excluded candidate files: ${result.excludedCount}`,
    "",
    markdownSafe(result.message),
  ];
  if (
    result.status === "screenshots_missing" &&
    result.candidatePaths?.length
  ) {
    lines.push(
      "",
      "Files requiring screenshots:",
      ...result.candidatePaths.map((pathname) => `- ${markdownEscaped(pathname)}`),
    );
  }
  return lines.join("\n");
}

function eventPullRequestNumber(event) {
  const value = event?.pull_request?.number ?? event?.inputs?.pr_number;
  const number = typeof value === "string" ? Number(value) : value;
  if (!Number.isInteger(number) || number < 1) {
    throw new Error("pull-request number is missing or invalid");
  }
  return number;
}

function runUrl(env) {
  if (!env.GITHUB_SERVER_URL || !env.GITHUB_REPOSITORY || !env.GITHUB_RUN_ID) {
    return undefined;
  }
  return `${env.GITHUB_SERVER_URL}/${env.GITHUB_REPOSITORY}/actions/runs/${encodeURIComponent(env.GITHUB_RUN_ID)}`;
}

async function writeSummary(summary, env, writer) {
  if (typeof writer === "function") {
    await writer(summary);
  } else if (
    typeof env.GITHUB_STEP_SUMMARY === "string" &&
    env.GITHUB_STEP_SUMMARY.length > 0
  ) {
    fs.appendFileSync(env.GITHUB_STEP_SUMMARY, summary, "utf8");
  }
}

async function run({
  client,
  env = process.env,
  event,
  eventName,
  writeSummary: summaryWriter,
  writeLog = (message) => process.stderr.write(`${message}\n`),
  publishStatus = env.PR_UI_EVIDENCE_DRY_RUN !== "1" &&
    env.PR_UI_EVIDENCE_DRY_RUN !== "true",
} = {}) {
  let pullNumber;
  let evaluatedHeadSha;
  let statusAttempted = false;
  let result;
  let apiClient = client;
  try {
    const effectiveEventName = eventName ?? env.GITHUB_EVENT_NAME;
    if (
      !["pull_request_target", "workflow_dispatch"].includes(effectiveEventName)
    ) {
      throw new Error(`unsupported GitHub event: ${effectiveEventName}`);
    }
    let effectiveEvent = event;
    if (!effectiveEvent) {
      if (
        typeof env.GITHUB_EVENT_PATH !== "string" ||
        env.GITHUB_EVENT_PATH.length === 0
      ) {
        throw new Error("GitHub event payload path is missing");
      }
      effectiveEvent = JSON.parse(
        fs.readFileSync(env.GITHUB_EVENT_PATH, "utf8"),
      );
    }
    pullNumber = eventPullRequestNumber(effectiveEvent);
    if (!apiClient) {
      const repository = /^([^/]+)\/([^/]+)$/.exec(env.GITHUB_REPOSITORY ?? "");
      if (!repository) {
        throw new Error("GITHUB_REPOSITORY is missing or invalid");
      }
      apiClient = new GitHubClient({
        owner: repository[1],
        repo: repository[2],
        token: env.GITHUB_TOKEN,
        trustedSha: env.TRUSTED_CHECKOUT_SHA,
      });
    }

    let snapshot = validatePullRequest(
      await apiClient.getPullRequest(pullNumber),
      pullNumber,
    );
    let stable = false;
    for (let attempt = 0; attempt < MAX_EVALUATION_ATTEMPTS; attempt += 1) {
      evaluatedHeadSha = snapshot.head.sha;
      result = await evaluateSnapshot({
        client: apiClient,
        pullRequest: snapshot,
        pullNumber,
      });
      const latest = validatePullRequest(
        await apiClient.getPullRequest(pullNumber),
        pullNumber,
      );
      if (sameMetadata(snapshot, latest)) {
        stable = true;
        break;
      }
      snapshot = latest;
    }
    if (!stable) {
      result = errorResult(
        "Pull request metadata changed during all evaluation attempts; retry the check.",
        pullNumber,
        evaluatedHeadSha,
      );
    }
    if (publishStatus && evaluatedHeadSha) {
      statusAttempted = true;
      await apiClient.createCommitStatus(
        evaluatedHeadSha,
        {
          description: STATUS_DESCRIPTIONS[result.status],
          state: statusState(result),
          targetUrl: runUrl(env),
        },
        STATUS_CONTEXT,
      );
    }
  } catch (error) {
    result = errorResult(error.message, pullNumber, evaluatedHeadSha);
    if (publishStatus && apiClient && evaluatedHeadSha && !statusAttempted) {
      statusAttempted = true;
      try {
        await apiClient.createCommitStatus(
          evaluatedHeadSha,
          {
            description: STATUS_DESCRIPTIONS.error,
            state: "error",
            targetUrl: runUrl(env),
          },
          STATUS_CONTEXT,
        );
      } catch {
        // The workflow failure remains visible if the status API is unavailable.
      }
    }
  }

  if (result.status === "error") {
    writeLog("UI screenshot evidence evaluation failed.");
  } else if (result.status === "screenshots_missing") {
    writeLog("UI screenshot evidence is missing from the Screenshots section.");
  }
  await writeSummary(resultSummary(result), env, summaryWriter);
  return { exitCode: statusState(result) === "success" ? 0 : 1, result };
}

if (require.main === module) {
  run()
    .then(({ exitCode }) => {
      process.exitCode = exitCode;
    })
    .catch((error) => {
      process.stderr.write(`${markdownSafe(error.message)}\n`);
      process.exitCode = 1;
    });
}

module.exports = {
  classifyUIChanges,
  evaluateSnapshot,
  hasScreenshotEmbed,
  run,
  screenshotSection,
  statusState,
};
