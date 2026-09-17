'use strict';

const path = require('node:path');
const fs = require('node:fs');
const { TextDecoder } = require('node:util');

const POSIX_PATH = path.posix;
const WORK_ORDER_PATTERN = /^docs\/plans\/[^/]+\/task-\d{2}-[^/]+\.md$/;
const LOCK_BASENAMES = new Set([
  'pnpm-lock.yaml',
  'package-lock.json',
  'yarn.lock',
  'go.sum',
  'Cargo.lock',
]);
const SCRIPT_TEST_EXTENSIONS = new Set([
  'cjs',
  'cts',
  'js',
  'jsx',
  'mjs',
  'mts',
  'ts',
  'tsx',
]);
const FRONTMATTER_FIELDS = new Set([
  'acceptance_criteria',
  'created',
  'depends_on',
  'id',
  'legacy_specs',
  'migration',
  'owners',
  'plan',
  'requirements',
  'status',
  'system',
  'system_design',
  'title',
  'updated',
  'last_updated',
  'specification_version',
  'wave',
]);
const ARRAY_FIELDS = new Set([
  'acceptance_criteria',
  'depends_on',
  'legacy_specs',
  'owners',
  'requirements',
  'system_design',
]);
const WORK_ORDER_REQUIRED_FIELDS = [
  'depends_on',
  'id',
  'title',
  'status',
  'wave',
  'plan',
  'requirements',
  'acceptance_criteria',
  'system_design',
];
const MAX_DOCUMENT_BYTES = 256 * 1024;
const MAX_DOCUMENTS = 100;
const MAX_TOTAL_DOCUMENT_BYTES = 4 * 1024 * 1024;
const MAX_RESPONSE_BYTES = 8 * 1024 * 1024;
const MAX_CHANGED_FILES = 3000;
const REQUEST_TIMEOUT_MS = 30_000;
const MAX_REQUEST_ATTEMPTS = 3;
const RETRY_BASE_DELAY_MS = 250;
const MAX_RETRY_SLEEP_MS = 180_000;
const SECONDARY_RATE_LIMIT_DELAYS_MS = [60_000, 120_000];
const NO_DOCS_LABEL = 'no-docs-allow';
const STATUS_CONTEXT = 'PR documentation coverage';
const GITHUB_API = 'https://api.github.com';

function defaultSleep(delay) {
  return new Promise(resolve => setTimeout(resolve, delay));
}

function defaultLog(message) {
  console.error(message);
}

function responseHeader(response, name) {
  if (typeof response?.headers?.get === 'function') {
    return response.headers.get(name) ?? undefined;
  }
  if (!response?.headers || typeof response.headers !== 'object') {
    return undefined;
  }
  const key = Object.keys(response.headers).find(header =>
    header.toLowerCase() === name.toLowerCase(),
  );
  return key === undefined ? undefined : String(response.headers[key]);
}

function retryAfterDelay(response) {
  const value = responseHeader(response, 'retry-after');
  if (value === undefined) {
    return undefined;
  }
  const trimmed = value.trim();
  if (/^\d+(?:\.\d+)?$/.test(trimmed)) {
    const seconds = Number(trimmed);
    return Number.isFinite(seconds) ? Math.max(0, seconds * 1000) : undefined;
  }
  const timestamp = Date.parse(trimmed);
  return Number.isFinite(timestamp) ? Math.max(0, timestamp - Date.now()) : undefined;
}

function primaryRateLimitResetDelay(response) {
  const remaining = Number(responseHeader(response, 'x-ratelimit-remaining'));
  if (!Number.isFinite(remaining) || remaining > 0) {
    return undefined;
  }
  const reset = Number(responseHeader(response, 'x-ratelimit-reset'));
  if (!Number.isFinite(reset) || reset <= 0) {
    return undefined;
  }
  return Math.max(0, reset * 1000 - Date.now());
}

function isGraphqlRateLimitPayload(payload) {
  return Array.isArray(payload?.errors)
    && payload.errors.some(error => error?.type === 'RATE_LIMITED');
}

function isRateLimitResponse(response, payload, requestClass) {
  if (requestClass === 'merge-queue' && isGraphqlRateLimitPayload(payload)) {
    return true;
  }
  if (response?.status === 429) {
    return true;
  }
  if (response?.status !== 403) {
    return false;
  }
  const remaining = Number(responseHeader(response, 'x-ratelimit-remaining'));
  return (
    (Number.isFinite(remaining) && remaining <= 0)
    || responseHeader(response, 'retry-after') !== undefined
    || /rate limit|secondary rate limit|abuse detection|temporarily unavailable/i.test(
      String(payload?.message ?? ''),
    )
  );
}

function isRetryableResponse(response, payload, requestClass) {
  const status = response?.status;
  return (
    status === 408
    || status === 429
    || (Number.isInteger(status) && status >= 500 && status <= 599)
    || isRateLimitResponse(response, payload, requestClass)
  );
}

function retryPlan(response, payload, attemptIndex, category, requestClass) {
  const rateLimited = isRateLimitResponse(response, payload, requestClass);
  const serverDelay = retryAfterDelay(response);
  if (serverDelay !== undefined) {
    return { delay: serverDelay, reason: 'server-wait' };
  }
  if (rateLimited) {
    const resetDelay = primaryRateLimitResetDelay(response);
    if (resetDelay !== undefined) {
      return { delay: resetDelay, reason: 'primary-rate-limit-reset' };
    }
    return {
      delay: SECONDARY_RATE_LIMIT_DELAYS_MS[attemptIndex] ?? MAX_RETRY_SLEEP_MS,
      reason: 'secondary-rate-limit-fallback',
    };
  }
  if (category === 'transport' || category === 'timeout' || isRetryableResponse(response, payload)) {
    return {
      delay: Math.min(RETRY_BASE_DELAY_MS * (2 ** attemptIndex), 5_000),
      reason: 'short-exponential-backoff',
    };
  }
  return undefined;
}

function requestClassForEndpoint(endpoint, method) {
  const pathname = endpoint.split('?', 1)[0];
  if (method === 'POST' && pathname.includes('/statuses/')) {
    return 'commit-status';
  }
  if (pathname.includes('/pulls/') && pathname.endsWith('/files')) {
    return 'changed-files';
  }
  if (pathname === '/search/code') {
    return 'code-search';
  }
  if (pathname === '/graphql') {
    return 'merge-queue';
  }
  if (pathname.includes('/pulls/')) {
    return 'pull-request';
  }
  return 'github-api';
}

function transportCategory(error) {
  return error?.name === 'TimeoutError' || error?.name === 'AbortError'
    ? 'timeout'
    : 'transport';
}

function normalizeRepoPath(value) {
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error('repository path must be a non-empty string');
  }
  if (value.includes('\0') || value.includes('\\') || value.startsWith('/')) {
    throw new Error(`unsafe repository path: ${value}`);
  }

  const normalized = POSIX_PATH.normalize(value);
  if (
    normalized === '.' ||
    normalized === '..' ||
    normalized.startsWith('../') ||
    normalized.includes('/../')
  ) {
    throw new Error(`repository path escapes the repository: ${value}`);
  }
  return normalized;
}

function pathExemption(pathname) {
  if (pathname === 'docs' || pathname.startsWith('docs/')) {
    return 'documentation tree';
  }
  if (pathname === 'plugin-registry/plugins.yaml') {
    return 'canonical plugin registry source';
  }
  if (/\.(?:md|mdx|markdown)$/i.test(pathname)) {
    return 'Markdown file';
  }
  if (/^\.codex\/agents\/[^/]+\.toml$/.test(pathname)) {
    return 'Codex agent definition';
  }
  if (pathname === '.codex/config.toml') {
    return 'Codex configuration';
  }
  if (pathname === '.claude/settings.json') {
    return 'Claude configuration';
  }
  if (/^\.cursor\/rules\/[^/]+\.mdc$/.test(pathname)) {
    return 'Cursor rule';
  }
  if (/_test\.go$/i.test(pathname)) {
    return 'Go test file';
  }
  if (pathname.startsWith('apps/web/e2e/')) {
    return 'web end-to-end test';
  }

  const basename = POSIX_PATH.basename(pathname);
  const extension = POSIX_PATH.extname(basename).slice(1).toLowerCase();
  if (
    /\.(?:test|spec)\.[^.]+$/i.test(basename) &&
    SCRIPT_TEST_EXTENSIONS.has(extension)
  ) {
    return 'JavaScript or TypeScript test file';
  }
  if (/^apps\/web\/src\/locales\/[^/]+\/[^/]+\.json$/.test(pathname)) {
    return 'web translation catalog';
  }
  if (LOCK_BASENAMES.has(basename)) {
    return 'dependency lock file';
  }
  return null;
}

function changedPaths(changedFiles) {
  const paths = [];
  const invalidPaths = [];
  const seen = new Set();
  for (const change of changedFiles ?? []) {
    if (typeof change !== 'string' && typeof change?.filename !== 'string') {
      invalidPaths.push('[missing filename]');
      continue;
    }
    const candidates = typeof change === 'string'
      ? [change]
      : [change.filename, change.previous_filename];
    for (const candidate of candidates) {
      if (candidate == null) {
        continue;
      }
      try {
        const normalized = normalizeRepoPath(candidate);
        if (!seen.has(normalized)) {
          seen.add(normalized);
          paths.push(normalized);
        }
      } catch (error) {
        invalidPaths.push(String(candidate));
      }
    }
  }
  return { paths, invalidPaths };
}

function classifyChangedFiles(changedFiles) {
  const { paths, invalidPaths } = changedPaths(changedFiles);
  const exemptPaths = [];
  const triggeringPaths = [...invalidPaths];
  const reasons = invalidPaths.map(pathname => ({
    path: pathname,
    reason: pathname === '[missing filename]' ? 'invalid changed-file record' : 'invalid repository path',
  }));

  for (const pathname of paths) {
    const reason = pathExemption(pathname);
    if (reason == null) {
      triggeringPaths.push(pathname);
      reasons.push({ path: pathname, reason: 'not in the exemption list' });
    } else {
      exemptPaths.push(pathname);
    }
  }

  return {
    changedPaths: paths,
    exemptPaths,
    invalidPaths,
    reasons,
    requiresCoverage: triggeringPaths.length > 0,
    triggeringPaths,
  };
}

function hasContentChanges(change) {
  if (change?.status !== 'renamed') {
    return change?.status !== 'removed';
  }
  if (Number.isFinite(change?.changes)) {
    return change.changes > 0;
  }
  return Number(change?.additions ?? 0) + Number(change?.deletions ?? 0) > 0;
}

function selectChangedWorkOrders(changedFiles) {
  const selected = [];
  const seen = new Set();
  for (const change of changedFiles ?? []) {
    const pathname = change?.filename ?? change;
    if (typeof pathname !== 'string') {
      continue;
    }
    let normalized;
    try {
      normalized = normalizeRepoPath(pathname);
    } catch {
      continue;
    }
    if (WORK_ORDER_PATTERN.test(normalized) && hasContentChanges(change)) {
      if (!seen.has(normalized)) {
        seen.add(normalized);
        selected.push(normalized);
      }
    }
  }
  return selected;
}

function parseScalar(value, field) {
  const trimmed = value.trim();
  if (trimmed.length === 0) {
    throw new Error(`frontmatter field ${field} must have a value`);
  }
  if (trimmed.startsWith('!') || trimmed.startsWith('&') || trimmed.startsWith('*')) {
    throw new Error(`frontmatter field ${field} uses unsupported YAML syntax`);
  }
  if (trimmed.startsWith('"')) {
    if (!trimmed.endsWith('"')) {
      throw new Error(`frontmatter field ${field} has an unterminated string`);
    }
    const parsed = JSON.parse(trimmed);
    if (typeof parsed !== 'string') {
      throw new Error(`frontmatter field ${field} must be a string`);
    }
    return parsed;
  }
  if (trimmed.startsWith("'")) {
    if (!trimmed.endsWith("'")) {
      throw new Error(`frontmatter field ${field} has an unterminated string`);
    }
    return trimmed.slice(1, -1).replaceAll("''", "'");
  }
  if (/[[\]{}|>]/.test(trimmed)) {
    throw new Error(`frontmatter field ${field} uses unsupported YAML syntax`);
  }
  if (/^-?\d+$/.test(trimmed)) {
    return Number(trimmed);
  }
  return trimmed;
}

function parseInlineArray(value, field) {
  const trimmed = value.trim();
  if (trimmed === '[]') {
    return [];
  }
  if (!trimmed.startsWith('[') || !trimmed.endsWith(']')) {
    throw new Error(`frontmatter field ${field} must be a list`);
  }
  const inner = trimmed.slice(1, -1).trim();
  if (inner.length === 0) {
    return [];
  }
  const values = [];
  let current = '';
  let quote = null;
  let escaped = false;
  for (const character of inner) {
    if (quote !== null) {
      current += character;
      if (escaped) {
        escaped = false;
      } else if (character === '\\' && quote === '"') {
        escaped = true;
      } else if (character === quote) {
        quote = null;
      }
    } else if (character === '"' || character === "'") {
      quote = character;
      current += character;
    } else if (character === ',') {
      values.push(parseScalar(current, field));
      current = '';
    } else {
      current += character;
    }
  }
  if (quote !== null) {
    throw new Error(`frontmatter field ${field} has an unterminated string`);
  }
  values.push(parseScalar(current, field));
  return values;
}

function parseFrontmatter(source) {
  if (typeof source !== 'string' || source.length === 0) {
    throw new Error('document is empty');
  }
  const lines = source.split(/\r?\n/);
  if (lines[0] !== '---') {
    throw new Error('frontmatter is missing');
  }

  const values = {};
  let currentList = null;
  let closingIndex = -1;
  for (let index = 1; index < lines.length; index += 1) {
    const line = lines[index];
    if (line === '---') {
      closingIndex = index;
      break;
    }
    if (line.trim().length === 0 || line.trim().startsWith('#')) {
      continue;
    }
    if (/^\s/.test(line)) {
      if (!currentList || !/^\s+-\s+/.test(line)) {
        throw new Error(`frontmatter line ${index + 1} is not a supported list item`);
      }
      const item = line.replace(/^\s+-\s+/, '');
      values[currentList].push(parseScalar(item, currentList));
      continue;
    }

    const match = /^([A-Za-z_][A-Za-z0-9_-]*):(?:[ \t]*(.*))?$/.exec(line);
    if (!match) {
      throw new Error(`frontmatter line ${index + 1} is invalid`);
    }
    const [, field, rawValue = ''] = match;
    if (!FRONTMATTER_FIELDS.has(field)) {
      throw new Error(`frontmatter field ${field} is unsupported`);
    }
    if (Object.hasOwn(values, field)) {
      throw new Error(`frontmatter field ${field} is duplicated`);
    }
    if (ARRAY_FIELDS.has(field)) {
      if (rawValue.trim().length === 0) {
        values[field] = [];
        currentList = field;
      } else {
        values[field] = parseInlineArray(rawValue, field);
        currentList = null;
      }
    } else {
      values[field] = parseScalar(rawValue, field);
      currentList = null;
    }
  }
  if (closingIndex < 0) {
    throw new Error('frontmatter closing delimiter is missing');
  }
  return {
    body: lines.slice(closingIndex + 1).join('\n'),
    data: values,
    frontmatter: lines.slice(1, closingIndex).join('\n'),
  };
}

function contentEntries(fileContents) {
  if (fileContents instanceof Map) {
    return [...fileContents.entries()];
  }
  if (fileContents && typeof fileContents === 'object') {
    return Object.entries(fileContents);
  }
  return [];
}

function contentFor(fileContents, pathname, deletedPaths) {
  if (deletedPaths.has(pathname)) {
    return undefined;
  }
  if (fileContents instanceof Map) {
    return fileContents.has(pathname) ? fileContents.get(pathname) : undefined;
  }
  return Object.hasOwn(fileContents ?? {}, pathname) ? fileContents[pathname] : undefined;
}

function resolveReference(basePath, reference) {
  if (typeof reference !== 'string' || reference.trim().length === 0) {
    throw new Error(`reference from ${basePath} is empty`);
  }
  if (reference.includes('\\') || reference.startsWith('/')) {
    throw new Error(`reference from ${basePath} is not repository-relative: ${reference}`);
  }
  const resolved = normalizeRepoPath(POSIX_PATH.join(POSIX_PATH.dirname(basePath), reference));
  return resolved;
}

function hasMarkdownLinkTo(planPath, body, targetPath) {
  const linkPattern = /\[[^\]]+\]\(\s*(?:<([^>]+)>|([^\s)]+))/g;
  for (const match of body.matchAll(linkPattern)) {
    const destination = (match[1] ?? match[2]).split(/[?#]/, 1)[0];
    if (destination.length === 0) {
      continue;
    }
    try {
      if (resolveReference(planPath, destination) === targetPath) {
        return true;
      }
    } catch {
      // Invalid links are not links to the selected work order.
    }
  }
  return false;
}

function asNonEmptyString(value, field, pathname) {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new Error(`${pathname} frontmatter field ${field} must be a non-empty string`);
  }
  return value;
}

function asNonEmptyStringList(value, field, pathname) {
  if (!Array.isArray(value) || value.length === 0) {
    throw new Error(`${pathname} frontmatter field ${field} must be a non-empty list`);
  }
  if (value.some(item => typeof item !== 'string' || item.trim().length === 0)) {
    throw new Error(`${pathname} frontmatter field ${field} contains an invalid item`);
  }
  return value;
}

function acceptanceCriteriaByRequirement(criteria, requirements, pathname, errors) {
  const referencedRequirements = new Set(requirements);
  const criteriaByRequirement = new Map();
  for (const criterion of criteria) {
    const match = /^AC-(.+)\.\d+$/.exec(criterion);
    const requirementId = match ? `REQ-${match[1]}` : null;
    if (!requirementId || !referencedRequirements.has(requirementId)) {
      errors.push(`${criterion} does not belong to a referenced requirement in ${pathname}`);
      continue;
    }
    const ownedCriteria = criteriaByRequirement.get(requirementId) ?? [];
    ownedCriteria.push(criterion);
    criteriaByRequirement.set(requirementId, ownedCriteria);
  }
  return criteriaByRequirement;
}

function requirementHeadingPattern(requirementId) {
  return new RegExp(`^#{1,6}\\s+${escapeRegExp(requirementId)}(?:\\s|:|$)`, 'm');
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function requirementDefinitions(fileContents, requirementId, system) {
  const prefix = `docs/specs/${system}/requirements/`;
  return contentEntries(fileContents)
    .filter(([pathname, content]) => {
      return (
        pathname.startsWith(prefix) &&
        pathname.endsWith('.md') &&
        typeof content === 'string' &&
        requirementHeadingPattern(requirementId).test(content)
      );
    })
    .map(([pathname, content]) => ({ pathname, content }));
}

function acceptanceCriteriaForRequirement(content, requirementId) {
  const lines = content.split(/\r?\n/);
  const heading = new RegExp(`^#{1,6}\\s+${escapeRegExp(requirementId)}(?:\\s|:|$)`);
  const start = lines.findIndex(line => heading.test(line));
  if (start < 0) {
    return [];
  }
  const level = (lines[start].match(/^#+/) ?? [''])[0].length;
  const section = [];
  for (let index = start + 1; index < lines.length; index += 1) {
    const nextHeading = lines[index].match(/^(#+)\s/);
    if (nextHeading && nextHeading[1].length <= level) {
      break;
    }
    section.push(lines[index]);
  }
  return [...new Set((section.join('\n').match(/\bAC-[A-Z0-9-]+\.\d+\b/g) ?? []))];
}

function deletedRepositoryPaths(changedFiles) {
  const deleted = new Set();
  for (const change of changedFiles ?? []) {
    if (change?.status === 'removed' || change?.status === 'renamed') {
      const pathname = change?.previous_filename;
      if (typeof pathname === 'string') {
        try {
          deleted.add(normalizeRepoPath(pathname));
        } catch {
          // Invalid changed paths are reported by classifyChangedFiles.
        }
      }
    }
  }
  return deleted;
}

function validateWorkOrder(workOrderPath, fileContents, deletedPaths) {
  const errors = [];
  const acceptedReferences = [];
  const workOrderContent = contentFor(fileContents, workOrderPath, deletedPaths);
  if (typeof workOrderContent !== 'string' || workOrderContent.trim().length === 0) {
    return {
      acceptedReferences,
      errors: [`${workOrderPath} is missing or empty`],
    };
  }
  if (Buffer.byteLength(workOrderContent, 'utf8') > MAX_DOCUMENT_BYTES) {
    return {
      acceptedReferences,
      errors: [`${workOrderPath} exceeds the ${MAX_DOCUMENT_BYTES}-byte document limit`],
    };
  }

  let workOrder;
  try {
    workOrder = parseFrontmatter(workOrderContent);
    for (const field of WORK_ORDER_REQUIRED_FIELDS) {
      if (!Object.hasOwn(workOrder.data, field)) {
        throw new Error(`${workOrderPath} is missing frontmatter field ${field}`);
      }
    }
    asNonEmptyString(workOrder.data.id, 'id', workOrderPath);
    asNonEmptyString(workOrder.data.title, 'title', workOrderPath);
    asNonEmptyString(workOrder.data.status, 'status', workOrderPath);
    if (!Number.isInteger(workOrder.data.wave) || workOrder.data.wave < 0) {
      throw new Error(`${workOrderPath} frontmatter field wave must be a non-negative integer`);
    }
    if (!Array.isArray(workOrder.data.depends_on)) {
      throw new Error(`${workOrderPath} frontmatter field depends_on must be a list`);
    }
    asNonEmptyStringList(workOrder.data.requirements, 'requirements', workOrderPath);
    asNonEmptyStringList(
      workOrder.data.acceptance_criteria,
      'acceptance_criteria',
      workOrderPath,
    );
    asNonEmptyStringList(workOrder.data.system_design, 'system_design', workOrderPath);
  } catch (error) {
    return { acceptedReferences, errors: [error.message] };
  }
  const criteriaByRequirement = acceptanceCriteriaByRequirement(
    workOrder.data.acceptance_criteria,
    workOrder.data.requirements,
    workOrderPath,
    errors,
  );

  let planPath;
  try {
    planPath = resolveReference(workOrderPath, workOrder.data.plan);
    const expectedPlanPath = `${POSIX_PATH.dirname(workOrderPath)}/plan.md`;
    if (planPath !== expectedPlanPath) {
      throw new Error(
        `${workOrderPath} plan reference resolves to ${planPath}; expected sibling plan.md`,
      );
    }
  } catch (error) {
    errors.push(error.message);
  }

  if (planPath) {
    const planContent = contentFor(fileContents, planPath, deletedPaths);
    if (typeof planContent !== 'string' || planContent.trim().length === 0) {
      errors.push(`${workOrderPath} references missing or empty ${planPath}`);
    } else if (Buffer.byteLength(planContent, 'utf8') > MAX_DOCUMENT_BYTES) {
      errors.push(`${planPath} exceeds the ${MAX_DOCUMENT_BYTES}-byte document limit`);
    } else {
      try {
        const plan = parseFrontmatter(planContent);
        if (!hasMarkdownLinkTo(planPath, plan.body, workOrderPath)) {
          errors.push(`${planPath} does not link back to ${workOrderPath}`);
        }
        const planRequirements = Array.isArray(plan.data.requirements)
          ? plan.data.requirements
          : [];
        for (const requirementId of workOrder.data.requirements) {
          if (!planRequirements.includes(requirementId)) {
            errors.push(`${planPath} does not cover requirement ${requirementId}`);
          }
        }
        const planDesignPaths = new Set();
        for (const reference of Array.isArray(plan.data.system_design)
          ? plan.data.system_design
          : []) {
          try {
            planDesignPaths.add(resolveReference(planPath, reference));
          } catch (error) {
            errors.push(error.message);
          }
        }
        const declaredRequirements = new Set();
        for (const reference of workOrder.data.system_design) {
          try {
            const designPath = resolveReference(workOrderPath, reference);
            if (!planDesignPaths.has(designPath)) {
              errors.push(`${planPath} does not cover system design ${designPath}`);
            }
            const designContent = contentFor(fileContents, designPath, deletedPaths);
            if (typeof designContent !== 'string' || designContent.trim().length === 0) {
              errors.push(`${workOrderPath} references missing or empty ${designPath}`);
              continue;
            }
            if (Buffer.byteLength(designContent, 'utf8') > MAX_DOCUMENT_BYTES) {
              errors.push(`${designPath} exceeds the ${MAX_DOCUMENT_BYTES}-byte document limit`);
              continue;
            }
            const design = parseFrontmatter(designContent);
            const designMatch = /^docs\/specs\/([^/]+)\/system-design\/[^/]+\.md$/.exec(
              designPath,
            );
            if (!designMatch) {
              errors.push(`${designPath} is outside the supported system-design directory`);
              continue;
            }
            const system = designMatch[1];
            const designRequirements = Array.isArray(design.data.requirements)
              ? design.data.requirements
              : [];
            const ownedRequirements = workOrder.data.requirements.filter(requirementId =>
              designRequirements.includes(requirementId)
            );
            for (const requirementId of ownedRequirements) {
              declaredRequirements.add(requirementId);
              const definitions = requirementDefinitions(fileContents, requirementId, system);
              if (definitions.length === 0) {
                errors.push(`${requirementId} is not defined in ${system} requirements`);
              } else if (definitions.length > 1) {
                errors.push(`${requirementId} has ambiguous definitions in ${system} requirements`);
              } else {
                const definedCriteria = acceptanceCriteriaForRequirement(
                  definitions[0].content,
                  requirementId,
                );
                for (const criterion of criteriaByRequirement.get(requirementId) ?? []) {
                  if (!definedCriteria.includes(criterion)) {
                    errors.push(
                      `${criterion} is not defined under ${requirementId} in ${definitions[0].pathname}`,
                    );
                  }
                }
              }
            }
            acceptedReferences.push({ designPath, requirements: ownedRequirements });
          } catch (error) {
            errors.push(error.message);
          }
        }
        for (const requirementId of workOrder.data.requirements) {
          if (!declaredRequirements.has(requirementId)) {
            errors.push(
              `${workOrderPath} requirement ${requirementId} is not declared by any referenced system design`,
            );
          }
        }
      } catch (error) {
        errors.push(`${planPath}: ${error.message}`);
      }
    }
  }

  return { acceptedReferences, errors };
}

function validateCoverage({ changedFiles = [], fileContents = {} } = {}) {
  const classification = classifyChangedFiles(changedFiles);
  const result = {
    acceptedReferences: [],
    changedPaths: classification.changedPaths,
    errors: [],
    exemptPaths: classification.exemptPaths,
    ok: false,
    remediation: [],
    requiresCoverage: classification.requiresCoverage,
    status: 'error',
    triggeringPaths: classification.triggeringPaths,
    workOrders: [],
  };

  if (!classification.requiresCoverage) {
    result.ok = true;
    result.status = 'exempt';
    result.remediation = ['No delivery package is required for these changed paths.'];
    return result;
  }
  for (const pathname of classification.invalidPaths) {
    result.errors.push(`Changed path ${pathname} is invalid`);
  }

  result.workOrders = selectChangedWorkOrders(changedFiles);
  if (result.workOrders.length > MAX_DOCUMENTS) {
    result.errors.push(`More than ${MAX_DOCUMENTS} changed work orders were supplied`);
  }
  if (result.workOrders.length === 0) {
    result.status = 'missing';
    result.errors.push(
      'No changed work order covers the triggering paths. Add docs/plans/<initiative>/task-NN-<slug>.md with linked plan, requirements, acceptance criteria, and system design.',
    );
  }

  const deletedPaths = deletedRepositoryPaths(changedFiles);
  let totalBytes = 0;
  for (const [pathname] of contentEntries(fileContents)) {
    const content = contentFor(fileContents, pathname, deletedPaths);
    if (typeof content === 'string') {
      totalBytes += Buffer.byteLength(content, 'utf8');
    }
  }
  if (totalBytes > MAX_TOTAL_DOCUMENT_BYTES) {
    result.errors.push(`Referenced documents exceed the ${MAX_TOTAL_DOCUMENT_BYTES}-byte total limit`);
  }
  for (const workOrderPath of result.workOrders) {
    const validation = validateWorkOrder(
      workOrderPath,
      fileContents,
      deletedPaths,
    );
    result.acceptedReferences.push(...validation.acceptedReferences);
    result.errors.push(...validation.errors);
  }
  if (result.errors.length === 0) {
    result.ok = true;
    result.status = 'covered';
    result.remediation = ['Keep the linked work order and referenced contracts available at this revision.'];
  } else {
    result.status = result.status === 'missing' ? 'missing' : 'invalid';
    result.remediation = [
      'Add or correct a changed work order and its complete plan, requirement, acceptance, and system-design references.',
    ];
  }
  return result;
}

function requireCommitSha(value, description) {
  if (typeof value !== 'string' || !/^[0-9a-f]{40}$/i.test(value)) {
    throw new Error(`${description} is missing or invalid`);
  }
  return value;
}

function requireBranchName(value, description) {
  if (typeof value !== 'string' || value.trim().length === 0) {
    throw new Error(`${description} is missing or invalid`);
  }
  const branch = value.startsWith('refs/heads/') ? value.slice('refs/heads/'.length) : value;
  if (
    branch.length === 0
    || branch.includes('\0')
    || /[\r\n]/.test(branch)
    || branch.startsWith('refs/')
  ) {
    throw new Error(`${description} is missing or invalid`);
  }
  return branch;
}

function requirePullRequestNumber(value) {
  if (!Number.isInteger(value) || value < 1) {
    throw new Error('pull-request number is missing or invalid');
  }
  return value;
}

class GitHubClient {
  constructor({
    owner,
    repo,
    token,
    fetchImpl = globalThis.fetch,
    sleepImpl = defaultSleep,
    logImpl = defaultLog,
  } = {}) {
    if (typeof owner !== 'string' || typeof repo !== 'string' || owner === '' || repo === '') {
      throw new Error('GitHub repository owner and name are required');
    }
    if (typeof token !== 'string' || token.length === 0) {
      throw new Error('GitHub token is required');
    }
    if (typeof fetchImpl !== 'function') {
      throw new Error('fetch implementation is required');
    }
    if (typeof sleepImpl !== 'function') {
      throw new Error('sleep implementation is required');
    }
    if (typeof logImpl !== 'function') {
      throw new Error('log implementation is required');
    }
    this.owner = owner;
    this.repo = repo;
    this.token = token;
    this.fetchImpl = fetchImpl;
    this.sleepImpl = sleepImpl;
    this.logImpl = logImpl;
    this.sleptMs = 0;
  }

  async request(endpoint, { method = 'GET', body, requestClass: requestedClass } = {}) {
    if (typeof endpoint !== 'string' || !endpoint.startsWith('/')) {
      throw new Error('GitHub endpoint must be an absolute API path');
    }
    const requestClass = typeof requestedClass === 'string' && requestedClass.length > 0
      ? requestedClass
      : requestClassForEndpoint(endpoint, method);
    const headers = {
      Accept: 'application/vnd.github+json',
      Authorization: `Bearer ${this.token}`,
      'X-GitHub-Api-Version': '2022-11-28',
    };
    if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    const statusForLog = response => Number.isInteger(response?.status)
      ? `HTTP ${response.status}`
      : 'none';
    const log = message => {
      try {
        this.logImpl(message);
      } catch {
        // Diagnostics must not hide the request result.
      }
    };
    const terminalError = ({
      category,
      response,
      attempt,
      outcome,
      message,
      nextDelay,
    }) => {
      const status = statusForLog(response);
      const diagnostic = [
        `class=${requestClass}`,
        `category=${category}`,
        `status=${status}`,
        `attempts=${attempt}`,
        `outcome=${outcome}`,
        ...(nextDelay === undefined ? [] : [`next_delay_ms=${nextDelay}`]),
      ].join(' ');
      log(`GitHub API request stopped: ${diagnostic}`);
      if (message) {
        return new Error(`${message} (${diagnostic})`);
      }
      return new Error(`GitHub API request failed (${diagnostic})`);
    };
    const waitBeforeRetry = async ({ response, category, attempt, plan }) => {
      const remainingMs = MAX_RETRY_SLEEP_MS - this.sleptMs;
      if (plan.delay > remainingMs) {
        return false;
      }
      log(
        `GitHub API request retry: class=${requestClass} category=${category} `
        + `status=${statusForLog(response)} attempt=${attempt + 1}/${MAX_REQUEST_ATTEMPTS} `
        + `delay_ms=${plan.delay} reason=${plan.reason}`,
      );
      try {
        await this.sleepImpl(plan.delay);
      } catch {
        throw terminalError({
          category: 'sleep',
          response,
          attempt: attempt + 1,
          outcome: 'retry-aborted',
        });
      }
      this.sleptMs += plan.delay;
      return true;
    };

    for (let attempt = 0; attempt < MAX_REQUEST_ATTEMPTS; attempt += 1) {
      const timeoutSignal = typeof AbortSignal === 'function'
        && typeof AbortSignal.timeout === 'function'
        ? AbortSignal.timeout(REQUEST_TIMEOUT_MS)
        : undefined;
      let response;
      try {
        response = await this.fetchImpl(`${GITHUB_API}${endpoint}`, {
          method,
          headers,
          body: body === undefined ? undefined : JSON.stringify(body),
          ...(timeoutSignal ? { signal: timeoutSignal } : {}),
        });
      } catch (error) {
        const category = transportCategory(error);
        const plan = retryPlan(undefined, undefined, attempt, category, requestClass);
        if (attempt + 1 < MAX_REQUEST_ATTEMPTS && plan) {
          if (await waitBeforeRetry({ category, attempt, plan })) {
            continue;
          }
          throw terminalError({
            category,
            attempt: attempt + 1,
            outcome: 'wait-budget-exhausted',
            nextDelay: plan.delay,
          });
        }
        throw terminalError({
          category,
          attempt: attempt + 1,
          message: category === 'timeout'
            ? `GitHub API request timed out after ${REQUEST_TIMEOUT_MS}ms`
            : undefined,
          outcome: 'retry-exhausted',
        });
      }
      if (!response || typeof response.text !== 'function') {
        throw terminalError({
          category: 'protocol',
          attempt: attempt + 1,
          message: 'GitHub API returned an invalid response',
          outcome: 'permanent',
        });
      }

      let text;
      try {
        text = await response.text();
      } catch (error) {
        const retryable = response.ok || isRetryableResponse(response, undefined, requestClass);
        const category = retryable ? transportCategory(error) : 'http';
        const plan = retryable
          ? retryPlan(response, undefined, attempt, category, requestClass)
          : undefined;
        if (retryable && attempt + 1 < MAX_REQUEST_ATTEMPTS && plan) {
          if (await waitBeforeRetry({ response, category, attempt, plan })) {
            continue;
          }
          throw terminalError({
            category,
            response,
            attempt: attempt + 1,
            outcome: 'wait-budget-exhausted',
            nextDelay: plan.delay,
          });
        }
        throw terminalError({
          category,
          response,
          attempt: attempt + 1,
          outcome: retryable ? 'retry-exhausted' : 'permanent',
        });
      }
      if (Buffer.byteLength(text, 'utf8') > MAX_RESPONSE_BYTES) {
        throw terminalError({
          category: 'response-too-large',
          response,
          attempt: attempt + 1,
          message: `GitHub API response exceeds the ${MAX_RESPONSE_BYTES}-byte limit`,
          outcome: 'permanent',
        });
      }
      let payload = null;
      if (text.length > 0) {
        try {
          payload = JSON.parse(text);
        } catch {
          const retryable = !response.ok && isRetryableResponse(response, undefined, requestClass);
          // The body is unavailable after parsing fails, so headers are the only signal.
          const category = isRateLimitResponse(response, undefined, requestClass)
            ? 'rate-limit'
            : response.status >= 500
              ? 'server'
              : 'protocol';
          const plan = retryable
            ? retryPlan(response, undefined, attempt, category, requestClass)
            : undefined;
          if (
            retryable
            && attempt + 1 < MAX_REQUEST_ATTEMPTS
            && plan
          ) {
            if (await waitBeforeRetry({ response, category, attempt, plan })) {
              continue;
            }
            throw terminalError({
              category,
              response,
              attempt: attempt + 1,
              outcome: 'wait-budget-exhausted',
              nextDelay: plan.delay,
            });
          }
          throw terminalError({
            category,
            response,
            attempt: attempt + 1,
            message: response.ok
              ? 'GitHub API returned invalid JSON'
              : `GitHub API request failed with HTTP ${response.status}: invalid JSON`,
            outcome: retryable ? 'retry-exhausted' : 'permanent',
          });
        }
      }
      if (response.ok && isRateLimitResponse(response, payload, requestClass)) {
        const category = 'rate-limit';
        const plan = retryPlan(response, payload, attempt, category, requestClass);
        if (attempt + 1 < MAX_REQUEST_ATTEMPTS && plan) {
          if (await waitBeforeRetry({ response, category, attempt, plan })) {
            continue;
          }
          throw terminalError({
            category,
            response,
            attempt: attempt + 1,
            outcome: 'wait-budget-exhausted',
            nextDelay: plan.delay,
          });
        }
        throw terminalError({
          category,
          response,
          attempt: attempt + 1,
          outcome: 'retry-exhausted',
        });
      }
      if (!response.ok) {
        const category = isRateLimitResponse(response, payload, requestClass)
          ? 'rate-limit'
          : response.status >= 500
            ? 'server'
            : 'http';
        const plan = retryPlan(response, payload, attempt, category, requestClass);
        if (
          attempt + 1 < MAX_REQUEST_ATTEMPTS
          && isRetryableResponse(response, payload, requestClass)
          && plan
        ) {
          if (await waitBeforeRetry({ response, category, attempt, plan })) {
            continue;
          }
          throw terminalError({
            category,
            response,
            attempt: attempt + 1,
            outcome: 'wait-budget-exhausted',
            nextDelay: plan.delay,
          });
        }
        throw terminalError({
          category,
          response,
          attempt: attempt + 1,
          message: `GitHub API request failed with HTTP ${response.status}`,
          outcome: isRetryableResponse(response, payload, requestClass)
            ? 'retry-exhausted'
            : 'permanent',
        });
      }
      return payload;
    }
  }

  async getPullRequest(number) {
    requirePullRequestNumber(number);
    const pullRequest = await this.request(
      `/repos/${encodeURIComponent(this.owner)}/${encodeURIComponent(this.repo)}/pulls/${number}`,
      { requestClass: 'pull-request' },
    );
    if (!pullRequest || pullRequest.number !== number) {
      throw new Error('GitHub returned a pull request with the wrong number');
    }
    if (typeof pullRequest.head?.sha !== 'string' || typeof pullRequest.base?.sha !== 'string') {
      throw new Error(`pull request #${number} has incomplete revision metadata`);
    }
    requireCommitSha(pullRequest.head.sha, `pull request #${number} head revision`);
    requireCommitSha(pullRequest.base.sha, `pull request #${number} base revision`);
    if (
      !Number.isInteger(pullRequest.changed_files) ||
      pullRequest.changed_files < 0 ||
      pullRequest.changed_files > MAX_CHANGED_FILES
    ) {
      throw new Error(`pull request #${number} has an invalid changed-file count`);
    }
    if (!Array.isArray(pullRequest.labels)) {
      throw new Error(`pull request #${number} has incomplete label metadata`);
    }
    return pullRequest;
  }

  async listFiles(number, expectedCount) {
    requirePullRequestNumber(number);
    const files = [];
    for (let page = 1; page <= 30; page += 1) {
      const pageFiles = await this.request(
        `/repos/${encodeURIComponent(this.owner)}/${encodeURIComponent(this.repo)}/pulls/${number}/files?per_page=100&page=${page}`,
        { requestClass: 'changed-files' },
      );
      if (!Array.isArray(pageFiles)) {
        throw new Error('GitHub changed-file response is not a list');
      }
      for (const file of pageFiles) {
        if (!file || typeof file.filename !== 'string' || file.filename.length === 0) {
          throw new Error('GitHub changed-file entry has no filename');
        }
        normalizeRepoPath(file.filename);
        if (!['added', 'changed', 'copied', 'deleted', 'modified', 'removed', 'renamed', 'unchanged'].includes(file.status)) {
          throw new Error(`GitHub changed-file entry for ${file.filename} has no supported status`);
        }
        if (file.status === 'renamed') {
          if (typeof file.previous_filename !== 'string' || file.previous_filename.length === 0) {
            throw new Error(`GitHub renamed file ${file.filename} has no previous filename`);
          }
          normalizeRepoPath(file.previous_filename);
        }
      }
      files.push(...pageFiles);
      if (files.length >= MAX_CHANGED_FILES) {
        throw new Error("GitHub changed-file response reaches the 3,000-file limit");
      }
      if (pageFiles.length < 100) {
        if (expectedCount !== undefined && expectedCount !== files.length) {
          throw new Error(
            `GitHub changed-file count ${files.length} does not match pull request metadata ${expectedCount}`,
          );
        }
        return files;
      }
    }
    throw new Error('GitHub changed-file pagination exceeded the supported limit');
  }

  async getFile(pathname, ref) {
    const normalized = normalizeRepoPath(pathname);
    if (typeof ref !== 'string' || ref.length === 0) {
      throw new Error(`content reference for ${normalized} is missing`);
    }
    const encodedPath = normalized.split('/').map(encodeURIComponent).join('/');
    const response = await this.request(
      `/repos/${encodeURIComponent(this.owner)}/${encodeURIComponent(this.repo)}/contents/${encodedPath}?ref=${encodeURIComponent(ref)}`,
      { requestClass: 'file-content' },
    );
    if (typeof response?.path !== 'string' || normalizeRepoPath(response.path) !== normalized) {
      throw new Error(`${normalized} response has a mismatched returned path`);
    }
    if (!response || response.type !== 'file' || response.encoding !== 'base64') {
      throw new Error(`${normalized} is not a regular base64-encoded file`);
    }
    if (typeof response.content !== 'string') {
      throw new Error(`${normalized} has no file content`);
    }
    const encoded = response.content.replace(/\s/g, '');
    if (!/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(encoded)) {
      throw new Error(`${normalized} has invalid base64 content`);
    }
    const bytes = Buffer.from(encoded, 'base64');
    if (bytes.length > MAX_DOCUMENT_BYTES) {
      throw new Error(`${normalized} exceeds the ${MAX_DOCUMENT_BYTES}-byte document limit`);
    }
    try {
      return new TextDecoder('utf-8', { fatal: true }).decode(bytes);
    } catch {
      throw new Error(`${normalized} is not valid UTF-8 text`);
    }
  }

  async listDirectory(pathname, ref) {
    const normalized = normalizeRepoPath(pathname);
    const encodedPath = normalized.split('/').map(encodeURIComponent).join('/');
    const response = await this.request(
      `/repos/${encodeURIComponent(this.owner)}/${encodeURIComponent(this.repo)}/contents/${encodedPath}?ref=${encodeURIComponent(ref)}`,
      { requestClass: 'directory-listing' },
    );
    if (!Array.isArray(response)) {
      throw new Error(`${normalized} is not a directory listing`);
    }
    return response.map(entry => {
      if (!entry || typeof entry.path !== 'string') {
        throw new Error(`${normalized} contains an invalid directory entry`);
      }
      const entryPath = normalizeRepoPath(entry.path);
      if (!['file', 'directory'].includes(entry.type)) {
        throw new Error(`${entryPath} is not a regular file or directory`);
      }
      return { path: entryPath, type: entry.type };
    });
  }

  async searchCode(text, directory) {
    if (typeof text !== 'string' || text.trim().length === 0) {
      throw new Error('GitHub code-search text is missing');
    }
    const normalizedDirectory = normalizeRepoPath(directory);
    const query = `"${text}" repo:${this.owner}/${this.repo} path:${normalizedDirectory}`;
    const response = await this.request(
      `/search/code?q=${encodeURIComponent(query)}&per_page=100`,
      { requestClass: 'code-search' },
    );
    if (
      !response
      || !Array.isArray(response.items)
      || response.incomplete_results === true
      || !Number.isInteger(response.total_count)
      || response.total_count < response.items.length
      || response.total_count > 100
    ) {
      throw new Error('GitHub code-search response is incomplete or exceeds the supported limit');
    }
    const prefix = `${normalizedDirectory}/`;
    const paths = new Set();
    for (const item of response.items) {
      if (typeof item?.path !== 'string') {
        throw new Error('GitHub code-search entry has no path');
      }
      const itemPath = normalizeRepoPath(item.path);
      if (!itemPath.startsWith(prefix) || !itemPath.endsWith('.md')) {
        throw new Error(`GitHub code-search returned an unsupported path: ${itemPath}`);
      }
      paths.add(itemPath);
    }
    return [...paths];
  }

  async createCommitStatus(sha, status) {
    requireCommitSha(sha, 'status revision');
    if (!status || !['pending', 'success', 'failure', 'error'].includes(status.state)) {
      throw new Error('commit status has an invalid state');
    }
    return this.request(
      `/repos/${encodeURIComponent(this.owner)}/${encodeURIComponent(this.repo)}/statuses/${sha}`,
      {
        method: 'POST',
        requestClass: 'commit-status',
        body: {
          state: status.state,
          context: STATUS_CONTEXT,
          description: String(status.description ?? '').slice(0, 140),
          target_url: status.targetUrl,
        },
      },
    );
  }

  async graphql(query, variables) {
    const response = await this.request('/graphql', {
      method: 'POST',
      requestClass: 'merge-queue',
      body: { query, variables },
    });
    if (Array.isArray(response?.errors) && response.errors.length > 0) {
      throw new Error(`GitHub GraphQL request failed: ${String(response.errors[0].message ?? 'unknown error')}`);
    }
    return response?.data;
  }

  async listMergeQueueEntries(branch) {
    const targetBranch = requireBranchName(branch, 'merge-queue target branch');
    const query = `
      query($owner: String!, $repo: String!, $branch: String!, $after: String) {
        repository(owner: $owner, name: $repo) {
          mergeQueue(branch: $branch) {
            entries(first: 100, after: $after) {
              nodes {
                baseCommit { oid }
                headCommit { oid }
                pullRequest { number headRefOid }
              }
              pageInfo { hasNextPage endCursor }
            }
          }
        }
      }
    `;
    const entries = [];
    let after = null;
    for (let page = 0; page < 10; page += 1) {
      const data = await this.graphql(query, {
        branch: targetBranch,
        owner: this.owner,
        repo: this.repo,
        after,
      });
      const connection = data?.repository?.mergeQueue?.entries;
      if (!connection || !Array.isArray(connection.nodes) || !connection.pageInfo) {
        throw new Error('GitHub merge queue response is incomplete');
      }
      entries.push(...connection.nodes);
      if (!connection.pageInfo.hasNextPage) {
        return entries;
      }
      if (typeof connection.pageInfo.endCursor !== 'string') {
        throw new Error('GitHub merge queue pagination cursor is missing');
      }
      after = connection.pageInfo.endCursor;
    }
    throw new Error('GitHub merge queue pagination exceeded the supported limit');
  }
}

function isNoDocsOverride(pullRequest) {
  return Array.isArray(pullRequest?.labels)
    && pullRequest.labels.some(label => label?.name === NO_DOCS_LABEL);
}

function resultMetadata(pullRequest) {
  return {
    baseSha: pullRequest.base.sha,
    baseRef: pullRequest.base.ref,
    headSha: pullRequest.head.sha,
    labels: pullRequest.labels.map(label => label?.name).filter(name => typeof name === 'string').sort(),
  };
}

function sameMetadata(left, right) {
  return (
    left.headSha === right.headSha &&
    left.baseSha === right.baseSha &&
    left.baseRef === right.baseRef &&
    JSON.stringify(left.labels) === JSON.stringify(right.labels)
  );
}

function errorCoverageResult(message, metadata = {}) {
  return {
    acceptedReferences: [],
    changedPaths: [],
    errors: [message],
    exemptPaths: [],
    baseRef: metadata.baseRef,
    headSha: metadata.headSha,
    ok: false,
    remediation: ['Retry the workflow after GitHub returns complete, stable revision and label data.'],
    requiresCoverage: true,
    status: 'error',
    triggeringPaths: [],
    workOrders: [],
  };
}

function isMissingResourceError(error) {
  return /\bHTTP 404\b/.test(String(error?.message ?? error));
}

function hasRequirementHeading(content, requirementId) {
  return typeof content === 'string' && requirementHeadingPattern(requirementId).test(content);
}

function changedRequirementSources(changedFiles, requirementDirectory) {
  const sources = new Map();
  const prefix = `${requirementDirectory}/`;
  for (const change of changedFiles ?? []) {
    const currentPath = typeof change === 'string' ? change : change?.filename;
    if (typeof currentPath !== 'string') {
      continue;
    }
    let normalizedCurrent;
    try {
      normalizedCurrent = normalizeRepoPath(currentPath);
    } catch {
      continue;
    }
    const status = typeof change === 'string' ? undefined : change?.status;
    const headPath = status === 'removed' ? undefined : normalizedCurrent;
    let basePath = normalizedCurrent;
    if (status === 'renamed' && typeof change.previous_filename === 'string') {
      try {
        basePath = normalizeRepoPath(change.previous_filename);
      } catch {
        basePath = undefined;
      }
    }
    const normalizedHead = headPath && headPath.startsWith(prefix) && headPath.endsWith('.md')
      ? headPath
      : undefined;
    const normalizedBase = basePath && basePath.startsWith(prefix) && basePath.endsWith('.md')
      ? basePath
      : undefined;
    if (!normalizedHead && !normalizedBase) {
      continue;
    }
    const key = normalizedHead ?? `base:${normalizedBase}`;
    if (!sources.has(key)) {
      sources.set(key, { basePath: normalizedBase, headPath: normalizedHead });
    }
  }
  return [...sources.values()];
}

async function loadCoverageContents({ client, changedFiles, headSha, baseSha }) {
  if (!classifyChangedFiles(changedFiles).requiresCoverage) {
    return {};
  }
  const contents = {};
  const loaded = new Map();
  const requirementSearches = new Map();
  const requirementDirectories = new Map();
  let documentCount = 0;
  let totalBytes = 0;
  async function load(pathname, ref, targetContents) {
    const normalized = normalizeRepoPath(pathname);
    const cacheKey = `${ref}\u0000${normalized}`;
    if (loaded.has(cacheKey)) {
      const content = loaded.get(cacheKey);
      if (targetContents) {
        targetContents[normalized] = content;
      }
      return content;
    }
    if (documentCount >= MAX_DOCUMENTS) {
      throw new Error(`Referenced documents exceed the ${MAX_DOCUMENTS}-document limit`);
    }
    let content;
    try {
      content = await client.getFile(normalized, ref);
    } catch (error) {
      if (!isMissingResourceError(error)) {
        throw error;
      }
    }
    loaded.set(cacheKey, content);
    documentCount += 1;
    if (targetContents) {
      targetContents[normalized] = content;
    }
    if (typeof content !== 'string' || content.trim().length === 0) {
      return undefined;
    }
    const size = Buffer.byteLength(content, 'utf8');
    if (size > MAX_DOCUMENT_BYTES) {
      throw new Error(`${normalized} exceeds the ${MAX_DOCUMENT_BYTES}-byte document limit`);
    }
    totalBytes += size;
    if (totalBytes > MAX_TOTAL_DOCUMENT_BYTES) {
      throw new Error(`Referenced documents exceed the ${MAX_TOTAL_DOCUMENT_BYTES}-byte total limit`);
    }
    return content;
  }

  for (const workOrderPath of selectChangedWorkOrders(changedFiles)) {
    const workOrderContent = await load(workOrderPath, headSha, contents);
    if (workOrderContent === undefined) {
      continue;
    }
    let workOrder;
    try {
      workOrder = parseFrontmatter(workOrderContent).data;
    } catch {
      continue;
    }
    let planPath;
    try {
      planPath = resolveReference(workOrderPath, workOrder.plan);
    } catch {
      continue;
    }
    const planContent = await load(planPath, headSha, contents);
    if (planContent === undefined) {
      continue;
    }
    let plan;
    try {
      plan = parseFrontmatter(planContent).data;
    } catch {
      continue;
    }
    const designReferences = [
      ...(Array.isArray(workOrder.system_design) ? workOrder.system_design : []),
      ...(Array.isArray(plan.system_design) ? plan.system_design : []),
    ];
    const designPaths = new Set();
    for (const reference of designReferences) {
      try {
        designPaths.add(resolveReference(workOrderPath, reference));
      } catch {
        // The validator reports the invalid reference; do not read outside the root.
      }
    }
    for (const designPath of designPaths) {
      const designContent = await load(designPath, headSha, contents);
      if (designContent === undefined) {
        continue;
      }
      let system;
      let designRequirements = [];
      try {
        const design = parseFrontmatter(designContent);
        designRequirements = Array.isArray(design.data.requirements)
          ? design.data.requirements
          : [];
        const match = /^docs\/specs\/([^/]+)\/system-design\/[^/]+\.md$/.exec(designPath);
        if (!match) {
          continue;
        }
        system = match[1];
      } catch {
        continue;
      }
      const requirementDirectory = `docs/specs/${system}/requirements`;
      const requirementPaths = new Set();
      const baseContentByHeadPath = new Map();
      for (const source of changedRequirementSources(changedFiles, requirementDirectory)) {
        if (source.headPath) {
          requirementPaths.add(source.headPath);
          await load(source.headPath, headSha, contents);
        }
        if (source.basePath) {
          const baseContent = await load(source.basePath, baseSha);
          if (source.headPath) {
            baseContentByHeadPath.set(source.headPath, baseContent);
          }
        }
      }
      const workOrderRequirements = new Set(workOrder.requirements ?? []);
      const referencedRequirementIds = designRequirements.filter(requirementId =>
        workOrderRequirements.has(requirementId)
      );
      const verifiedRequirementIds = new Set();
      for (const requirementId of referencedRequirementIds) {
        const definitions = requirementDefinitions(contents, requirementId, system);
        if (definitions.length !== 1) {
          continue;
        }
        const definition = definitions[0];
        const baseContent = baseContentByHeadPath.get(definition.pathname);
        if (
          hasRequirementHeading(definition.content, requirementId)
          && hasRequirementHeading(baseContent, requirementId)
        ) {
          verifiedRequirementIds.add(requirementId);
        }
      }
      const unresolvedRequirementIds = [];
      if (typeof client.searchCode === 'function') {
        for (const requirementId of referencedRequirementIds) {
          if (verifiedRequirementIds.has(requirementId)) {
            continue;
          }
          const searchKey = JSON.stringify([requirementDirectory, requirementId]);
          if (!requirementSearches.has(searchKey)) {
            requirementSearches.set(
              searchKey,
              await client.searchCode(requirementId, requirementDirectory),
            );
          }
          const matches = requirementSearches.get(searchKey);
          if (matches.length === 0) {
            unresolvedRequirementIds.push(requirementId);
          }
          for (const pathname of matches) {
            requirementPaths.add(pathname);
          }
        }
      } else {
        unresolvedRequirementIds.push(
          ...referencedRequirementIds.filter(requirementId => !verifiedRequirementIds.has(requirementId)),
        );
      }
      if (unresolvedRequirementIds.length > 0 && typeof client.listDirectory === 'function') {
        if (!requirementDirectories.has(requirementDirectory)) {
          let entries = [];
          try {
            entries = await client.listDirectory(requirementDirectory, headSha);
          } catch (error) {
            if (!isMissingResourceError(error)) {
              throw error;
            }
          }
          requirementDirectories.set(requirementDirectory, entries);
        }
        const entries = requirementDirectories.get(requirementDirectory);
        const candidateNames = new Set(unresolvedRequirementIds.map(requirementId => {
          const parts = requirementId.split('-');
          return `${parts.slice(2).join('-').replace(/-\d+$/, '').toLowerCase()}.md`;
        }));
        for (const entry of entries) {
          if (entry.type === 'file' && candidateNames.has(POSIX_PATH.basename(entry.path))) {
            requirementPaths.add(entry.path);
          }
        }
      }
      for (const pathname of requirementPaths) {
        const requirementContent = await load(pathname, headSha, contents);
        if (requirementContent === undefined) {
          continue;
        }
      }
    }
  }
  return contents;
}

async function evaluatePullRequestSnapshot({ client, pullRequest, expectedHeadSha }) {
  if (expectedHeadSha && pullRequest.head.sha !== expectedHeadSha) {
    return errorCoverageResult(
      `pull request #${pullRequest.number} head ${pullRequest.head.sha} does not match expected queue head ${expectedHeadSha}`,
      { headSha: pullRequest.head.sha },
    );
  }
  const metadata = {
    baseSha: pullRequest.base.sha,
    baseRef: pullRequest.base.ref,
    headSha: pullRequest.head.sha,
    pullRequestNumber: pullRequest.number,
  };
  if (isNoDocsOverride(pullRequest)) {
    return {
      acceptedReferences: [],
      changedPaths: [],
      errors: [],
      exemptPaths: [],
      ...metadata,
      ok: true,
      override: NO_DOCS_LABEL,
      remediation: ['The no-docs-allow label is present. Remove it to restore normal evaluation.'],
      requiresCoverage: false,
      status: 'override',
      triggeringPaths: [],
      workOrders: [],
    };
  }
  const changedFiles = await client.listFiles(pullRequest.number, pullRequest.changed_files);
  const fileContents = await loadCoverageContents({
    baseSha: pullRequest.base.sha,
    changedFiles,
    client,
    headSha: pullRequest.head.sha,
  });
  return {
    ...validateCoverage({ changedFiles, fileContents }),
    ...metadata,
  };
}

async function evaluatePullRequest({
  client,
  pullNumber,
  expectedHeadSha,
  maxAttempts = 3,
  initialPullRequest,
} = {}) {
  requirePullRequestNumber(pullNumber);
  if (!Number.isInteger(maxAttempts) || maxAttempts < 1 || maxAttempts > 5) {
    throw new Error('maxAttempts must be an integer from 1 through 5');
  }
  let lastMetadata;
  let firstSnapshot = initialPullRequest;
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    let pullRequest;
    try {
      pullRequest = firstSnapshot ?? await client.getPullRequest(pullNumber);
      firstSnapshot = undefined;
      const before = resultMetadata(pullRequest);
      if (expectedHeadSha && pullRequest.head.sha !== expectedHeadSha) {
        return errorCoverageResult(
          `pull request #${pullNumber} head does not match the expected revision`,
          before,
        );
      }
      const result = await evaluatePullRequestSnapshot({
        client,
        expectedHeadSha,
        pullRequest,
      });
      const latest = await client.getPullRequest(pullNumber);
      const after = resultMetadata(latest);
      if (sameMetadata(before, after)) {
        return result;
      }
      lastMetadata = after;
    } catch (error) {
      return errorCoverageResult(error.message, lastMetadata ?? {});
    }
  }
  return errorCoverageResult(
    `pull request #${pullNumber} changed during evaluation; retry limit reached`,
    lastMetadata ?? {},
  );
}

function validateMergeQueueEntries(entries) {
  if (!Array.isArray(entries) || entries.length === 0) {
    throw new Error('merge-group membership is missing');
  }
  return entries.map((entry, index) => {
    const entryBaseSha = requireCommitSha(entry?.baseCommit?.oid, `merge-queue entry ${index} base`);
    const entryHeadSha = requireCommitSha(entry?.headCommit?.oid, `merge-queue entry ${index} head`);
    const number = requirePullRequestNumber(entry?.pullRequest?.number);
    if (entryBaseSha === entryHeadSha) {
      throw new Error(`merge-queue entry ${number} has identical base and head revisions`);
    }
    return {
      baseSha: entryBaseSha,
      headSha: entryHeadSha,
      number,
      pullRequest: entry.pullRequest,
      raw: entry,
    };
  });
}

function resolveMergeGroupMembers({ baseSha, headSha, entries } = {}) {
  requireCommitSha(baseSha, 'merge-group base revision');
  requireCommitSha(headSha, 'merge-group head revision');
  const normalizedEntries = validateMergeQueueEntries(entries);

  const members = [];
  const usedEntries = new Set();
  const usedPullRequests = new Set();
  let current = headSha;
  while (current !== baseSha) {
    const candidates = normalizedEntries.filter(entry => entry.headSha === current);
    if (candidates.length === 0) {
      throw new Error(`merge-group boundary ${current} is not connected to its base`);
    }
    if (candidates.length > 1) {
      throw new Error(`merge-group boundary ${current} has ambiguous membership`);
    }
    const entry = candidates[0];
    if (usedEntries.has(entry) || usedPullRequests.has(entry.number)) {
      throw new Error('merge-group membership contains a cycle or duplicate pull request');
    }
    usedEntries.add(entry);
    usedPullRequests.add(entry.number);
    members.push(entry);
    current = entry.baseSha;
    if (members.length > normalizedEntries.length) {
      throw new Error('merge-group membership exceeds the available queue entries');
    }
  }
  if (members.length === 0) {
    throw new Error('merge-group contains no pull requests');
  }
  members.reverse();
  return { baseSha, headSha, members };
}

async function evaluateMergeGroup({ client, baseSha, headSha, entries, maxAttempts = 3 } = {}) {
  const group = resolveMergeGroupMembers({ baseSha, entries, headSha });
  const memberResults = [];
  for (const member of group.members) {
    const expectedHeadSha = requireCommitSha(
      member.pullRequest?.headRefOid,
      `merge-queue member ${member.number} pull-request head`,
    );
    const result = await evaluatePullRequest({
      client,
      expectedHeadSha,
      maxAttempts,
      pullNumber: member.number,
    });
    memberResults.push({
      ...result,
      baseSha: member.baseSha,
      expectedHeadSha,
      number: member.number,
    });
  }
  const errors = memberResults.flatMap(result => result.errors ?? []);
  return {
    baseSha,
    errors,
    headSha,
    memberResults,
    ok: memberResults.every(result => result.ok),
    status: memberResults.some(result => result.status === 'error')
      ? 'error'
      : memberResults.every(result => result.ok)
        ? 'success'
        : 'failure',
  };
}

function findAffectedMergeGroups({ entries, pullRequestNumber } = {}) {
  if (!Array.isArray(entries) || entries.length === 0) {
    return [];
  }
  if (pullRequestNumber !== undefined) {
    requirePullRequestNumber(pullRequestNumber);
  }
  const normalizedEntries = validateMergeQueueEntries(entries);
  const headRevisions = new Set(normalizedEntries.map(entry => entry.headSha));
  const terminalEntries = normalizedEntries.filter(entry => !normalizedEntries.some(
    other => other.baseSha === entry.headSha,
  ));
  if (terminalEntries.length === 0) {
    throw new Error('merge-queue entries do not have a terminal group boundary');
  }

  const groups = [];
  const consumed = new Set();
  for (const terminal of terminalEntries) {
    const chain = [];
    const chainEntries = new Set();
    let current = terminal.headSha;
    while (true) {
      const candidates = normalizedEntries.filter(entry => entry.headSha === current);
      if (candidates.length === 0) {
        throw new Error(`merge-group boundary ${current} is not connected to any queue entry`);
      }
      if (candidates.length > 1) {
        throw new Error(`merge-group boundary ${current} has ambiguous membership`);
      }
      const entry = candidates[0];
      if (chainEntries.has(entry) || consumed.has(entry)) {
        throw new Error('merge-queue entries overlap or contain a cycle');
      }
      chainEntries.add(entry);
      chain.unshift(entry);
      if (!headRevisions.has(entry.baseSha)) {
        break;
      }
      current = entry.baseSha;
    }
    for (const entry of chain) {
      consumed.add(entry);
    }
    for (let prefixLength = 1; prefixLength <= chain.length; prefixLength += 1) {
      const prefix = chain.slice(0, prefixLength);
      if (
        pullRequestNumber !== undefined &&
        !prefix.some(entry => entry.number === pullRequestNumber)
      ) {
        continue;
      }
      groups.push({
        baseSha: prefix[0].baseSha,
        entries: prefix.map(entry => entry.raw),
        headSha: prefix[prefix.length - 1].headSha,
      });
    }
  }
  if (consumed.size !== normalizedEntries.length) {
    throw new Error('merge-queue membership contains an unconnected entry');
  }
  return groups;
}

function statusState(result) {
  if (result?.status === 'error') {
    return 'error';
  }
  return result?.ok ? 'success' : 'failure';
}

function markdownValue(value) {
  return String(value)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
    .replaceAll('`', '&#96;')
    .replace(/[\r\n]/g, ' ');
}

function resultSummary(result) {
  const lines = [`## PR documentation coverage`, `Result: **${markdownValue(result.status)}**`];
  if (result.override) {
    lines.push(`Override: \`${markdownValue(result.override)}\``);
  }
  const reportedPaths = result.triggeringPaths?.length > 0
    ? result.triggeringPaths
    : result.changedPaths ?? [];
  if (reportedPaths.length > 0) {
    lines.push('', result.triggeringPaths?.length > 0 ? 'Triggering paths:' : 'Changed paths:');
    for (const pathname of reportedPaths) {
      lines.push(`- \`${markdownValue(pathname)}\``);
    }
  }
  if (result.workOrders?.length > 0) {
    lines.push('', 'Accepted work orders:');
    for (const pathname of result.workOrders) {
      lines.push(`- \`${markdownValue(pathname)}\``);
    }
  }
  if (result.acceptedReferences?.length > 0) {
    lines.push('', 'Accepted references:');
    for (const reference of result.acceptedReferences) {
      lines.push(`- \`${markdownValue(reference.designPath)}\``);
    }
  }
  if (result.errors?.length > 0) {
    lines.push('', 'Problems:');
    for (const error of result.errors) {
      lines.push(`- ${markdownValue(error)}`);
    }
  }
  if (result.remediation?.length > 0) {
    lines.push('', 'Next steps:');
    for (const remediation of result.remediation) {
      lines.push(`- ${markdownValue(remediation)}`);
    }
  }
  if (result.memberResults?.length > 0) {
    lines.push('', 'Merge-group members:');
    for (const member of result.memberResults) {
      lines.push(`- #${member.number}: **${markdownValue(member.status)}**`);
    }
  }
  return `${lines.join('\n')}\n`;
}

function statusDescription(result) {
  if (result.override) {
    return `Override: ${NO_DOCS_LABEL}`;
  }
  if (result.status === 'exempt') {
    return 'All changed paths are exempt';
  }
  if (result.status === 'covered') {
    return 'Linked delivery package found';
  }
  if (result.status === 'missing') {
    return 'Linked delivery package is missing';
  }
  if (result.status === 'invalid') {
    return 'Linked delivery package is incomplete';
  }
  if (result.status === 'success') {
    return 'Every merge-group member satisfies documentation coverage';
  }
  if (result.status === 'failure') {
    return 'A merge-group member does not satisfy documentation coverage';
  }
  return 'Coverage evaluation failed';
}

function runUrl(env) {
  if (!env.GITHUB_SERVER_URL || !env.GITHUB_REPOSITORY || !env.GITHUB_RUN_ID) {
    return undefined;
  }
  return `${env.GITHUB_SERVER_URL}/${env.GITHUB_REPOSITORY}/actions/runs/${encodeURIComponent(env.GITHUB_RUN_ID)}`;
}

function eventPullRequestNumber(event) {
  const value = event?.pull_request?.number ?? event?.inputs?.pr_number;
  const number = typeof value === 'string' ? Number(value) : value;
  return requirePullRequestNumber(number);
}

async function writeRunSummary(summary, env, writeSummary) {
  if (typeof writeSummary === 'function') {
    await writeSummary(summary);
    return;
  }
  if (typeof env.GITHUB_STEP_SUMMARY === 'string' && env.GITHUB_STEP_SUMMARY.length > 0) {
    fs.appendFileSync(env.GITHUB_STEP_SUMMARY, summary, 'utf8');
  }
}

async function publishResult(client, sha, result, targetUrl) {
  await client.createCommitStatus(sha, {
    description: statusDescription(result),
    state: statusState(result),
    targetUrl,
  });
}

async function evaluateAffectedGroups({ client, pullNumber, targetUrl, entries }) {
  const groups = findAffectedMergeGroups({ entries, pullRequestNumber: pullNumber });
  const groupResults = [];
  for (const group of groups) {
    await client.createCommitStatus(group.headSha, {
      description: 'Evaluating merge-group documentation coverage',
      state: 'pending',
      targetUrl,
    });
    const result = await evaluateMergeGroup({
      baseSha: group.baseSha,
      client,
      entries: group.entries,
      headSha: group.headSha,
    });
    await publishResult(client, group.headSha, result, targetUrl);
    groupResults.push(result);
  }
  return groupResults;
}

async function run({ client, env = process.env, event, eventName, writeSummary } = {}) {
  const effectiveEventName = eventName ?? env.GITHUB_EVENT_NAME;
  const effectiveEvent = event ?? (() => {
    if (typeof env.GITHUB_EVENT_PATH !== 'string' || env.GITHUB_EVENT_PATH.length === 0) {
      throw new Error('GitHub event payload path is missing');
    }
    return JSON.parse(fs.readFileSync(env.GITHUB_EVENT_PATH, 'utf8'));
  })();
  if (!['pull_request_target', 'workflow_dispatch', 'merge_group'].includes(effectiveEventName)) {
    throw new Error(`unsupported GitHub event: ${effectiveEventName}`);
  }

  let apiClient = client;
  if (!apiClient) {
    const repository = /^([^/]+)\/([^/]+)$/.exec(env.GITHUB_REPOSITORY ?? '');
    if (!repository) {
      throw new Error('GITHUB_REPOSITORY is missing or invalid');
    }
    apiClient = new GitHubClient({
      fetchImpl: globalThis.fetch,
      owner: repository[1],
      repo: repository[2],
      token: env.GITHUB_TOKEN,
    });
  }

  const targetUrl = runUrl(env);
  let pendingSha;
  let result;
  try {
    if (effectiveEventName === 'merge_group') {
      const targetBranch = requireBranchName(
        effectiveEvent.merge_group?.base_ref,
        'merge-group target branch',
      );
      const baseSha = requireCommitSha(
        effectiveEvent.merge_group?.base_sha,
        'merge-group base revision',
      );
      const headSha = requireCommitSha(
        effectiveEvent.merge_group?.head_sha,
        'merge-group head revision',
      );
      pendingSha = headSha;
      await apiClient.createCommitStatus(headSha, {
        description: 'Evaluating merge-group documentation coverage',
        state: 'pending',
        targetUrl,
      });
      const entries = await apiClient.listMergeQueueEntries(targetBranch);
      result = await evaluateMergeGroup({
        baseSha,
        client: apiClient,
        entries,
        headSha,
      });
      await publishResult(apiClient, headSha, result, targetUrl);
    } else {
      const pullNumber = eventPullRequestNumber(effectiveEvent);
      const current = await apiClient.getPullRequest(pullNumber);
      pendingSha = requireCommitSha(current.head.sha, 'pull-request head revision');
      await apiClient.createCommitStatus(pendingSha, {
        description: 'Evaluating pull-request documentation coverage',
        state: 'pending',
        targetUrl,
      });
      result = await evaluatePullRequest({
        client: apiClient,
        initialPullRequest: current,
        pullNumber,
      });
      await publishResult(apiClient, result.headSha ?? pendingSha, result, targetUrl);

      if (effectiveEventName !== 'workflow_dispatch') {
        const action = effectiveEvent.action;
        if (action === 'labeled' || action === 'unlabeled') {
          const targetBranch = requireBranchName(
            result.baseRef ?? current.base?.ref,
            'pull-request target branch',
          );
          const entries = await apiClient.listMergeQueueEntries(targetBranch);
          const groupResults = await evaluateAffectedGroups({
            client: apiClient,
            entries,
            pullNumber,
            targetUrl,
          });
          if (groupResults.some(groupResult => !groupResult.ok)) {
            result = {
              ...result,
              affectedGroups: groupResults,
              errors: [
                ...(result.errors ?? []),
                'An affected merge group does not satisfy documentation coverage.',
              ],
              ok: false,
              status: 'failure',
            };
          }
        }
      }
    }
  } catch (error) {
    result = errorCoverageResult(error.message, { headSha: pendingSha });
    if (pendingSha) {
      try {
        await publishResult(apiClient, pendingSha, result, targetUrl);
      } catch {
        // The original error remains the authoritative failure for the job.
      }
    }
  }

  await writeRunSummary(resultSummary(result), env, writeSummary);
  return { exitCode: result.ok ? 0 : 1, result };
}

if (require.main === module) {
  run()
    .then(({ exitCode }) => {
      process.exitCode = exitCode;
    })
    .catch(error => {
      process.stderr.write(`${error.message}\n`);
      process.exitCode = 1;
    });
}

module.exports = {
  classifyChangedFiles,
  evaluateMergeGroup,
  evaluatePullRequest,
  findAffectedMergeGroups,
  GitHubClient,
  isNoDocsOverride,
  normalizeRepoPath,
  parseFrontmatter,
  resolveMergeGroupMembers,
  resultSummary,
  run,
  selectChangedWorkOrders,
  statusState,
  validateCoverage,
};
