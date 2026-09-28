'use strict';

const { execFile } = require('node:child_process');
const os = require('node:os');
const path = require('node:path');
const { TextDecoder } = require('node:util');

const MAX_COMMAND_OUTPUT_BYTES = 1024 * 1024;
const MAX_FETCH_OUTPUT_BYTES = 64 * 1024;
const MAX_CANDIDATE_PATHS = 200;
const MAX_GIT_COMMAND_MS = 45_000;
const MAX_GIT_LOOKUP_MS = 180_000;
const SHA_PATTERN = /^[0-9a-f]{40}$/i;
const REQUIREMENT_ID_PATTERN = /^REQ-[A-Z0-9]+(?:-[A-Z0-9]+)*$/;
const REQUIREMENT_DIRECTORY_PATTERN = /^docs\/specs\/[a-z0-9][a-z0-9-]*\/requirements$/;
const utf8Decoder = new TextDecoder('utf-8', { fatal: true });

class GitLookupError extends Error {
  constructor(stage, detail) {
    super(`Git object lookup ${stage} failed (${detail})`);
    this.name = 'GitLookupError';
  }
}

function requireSha(value, label) {
  if (typeof value !== 'string' || !SHA_PATTERN.test(value)) {
    throw new GitLookupError(label, 'invalid commit ID');
  }
  return value.toLowerCase();
}

function requirePullNumber(value) {
  if (!Number.isSafeInteger(value) || value < 1) {
    throw new GitLookupError('fetch', 'invalid pull request number');
  }
  return value;
}

function validateSearch(directory, requirementId) {
  if (typeof directory !== 'string' || !REQUIREMENT_DIRECTORY_PATTERN.test(directory)) {
    throw new GitLookupError('search', 'invalid requirements directory');
  }
  if (typeof requirementId !== 'string' || !REQUIREMENT_ID_PATTERN.test(requirementId)) {
    throw new GitLookupError('search', 'invalid requirement ID');
  }
}

function childEnvironment() {
  const env = Object.fromEntries(
    Object.entries(process.env).filter(([key]) => !/^GIT_/i.test(key)),
  );
  Object.assign(env, {
    GIT_ATTR_NOSYSTEM: '1',
    GIT_CONFIG_GLOBAL: os.devNull,
    GIT_CONFIG_NOSYSTEM: '1',
    GIT_NO_REPLACE_OBJECTS: '1',
    GIT_OPTIONAL_LOCKS: '0',
    GIT_PAGER: 'cat',
    GIT_TERMINAL_PROMPT: '0',
  });
  return env;
}

function processFailureDetail(error) {
  if (error?.code === 'ERR_CHILD_PROCESS_STDIO_MAXBUFFER') {
    return 'output limit exceeded';
  }
  if (error?.code === 'ETIMEDOUT' || error?.killed || error?.signal) {
    return 'time limit exceeded';
  }
  return 'process could not start';
}

function executeGit(args, { cwd, timeoutMs, maxOutputBytes, stage }) {
  const commandArgs = [
    '-c', `core.hooksPath=${os.devNull}`,
    '-c', `core.attributesFile=${os.devNull}`,
    '-c', 'core.pager=cat',
    ...args,
  ];
  return new Promise((resolve, reject) => {
    execFile('git', commandArgs, {
      cwd,
      encoding: 'buffer',
      env: childEnvironment(),
      killSignal: 'SIGKILL',
      maxBuffer: maxOutputBytes,
      timeout: timeoutMs,
      windowsHide: true,
    }, (error, stdout, stderr) => {
      if (error && !Number.isInteger(error.code)) {
        reject(new GitLookupError(stage, processFailureDetail(error)));
        return;
      }
      resolve({
        code: error ? error.code : 0,
        stderr: Buffer.isBuffer(stderr) ? stderr : Buffer.from(stderr ?? ''),
        stdout: Buffer.isBuffer(stdout) ? stdout : Buffer.from(stdout ?? ''),
      });
    });
  });
}

function isSafeRepositoryPath(pathname) {
  return typeof pathname === 'string'
    && pathname.length > 0
    && !pathname.startsWith('/')
    && !pathname.includes('\\')
    && !/[\u0000-\u001f\u007f]/.test(pathname)
    && pathname.split('/').every(part => part.length > 0 && part !== '.' && part !== '..')
    && path.posix.normalize(pathname) === pathname;
}

function parseGitGrepPaths(stdout, headSha, directory) {
  if (stdout.length === 0) {
    return [];
  }
  if (stdout[stdout.length - 1] !== 0) {
    throw new GitLookupError('search', 'incomplete path output');
  }

  const prefix = Buffer.from(`${headSha}:`, 'ascii');
  const paths = new Set();
  let start = 0;
  while (start < stdout.length) {
    const end = stdout.indexOf(0, start);
    if (end < 0 || end === start) {
      throw new GitLookupError('search', 'malformed path output');
    }
    const record = stdout.subarray(start, end);
    if (!record.subarray(0, prefix.length).equals(prefix)) {
      throw new GitLookupError('search', 'path output has the wrong revision');
    }
    let pathname;
    try {
      pathname = utf8Decoder.decode(record.subarray(prefix.length));
    } catch {
      throw new GitLookupError('search', 'path output is not valid UTF-8');
    }
    if (
      !isSafeRepositoryPath(pathname)
      || !pathname.startsWith(`${directory}/`)
      || !pathname.endsWith('.md')
    ) {
      throw new GitLookupError('search', 'unsafe candidate path');
    }
    paths.add(pathname);
    if (paths.size > MAX_CANDIDATE_PATHS) {
      throw new GitLookupError('search', 'candidate limit exceeded');
    }
    start = end + 1;
  }
  return [...paths];
}

class GitHeadReader {
  constructor({
    cwd = process.cwd(),
    trustedSha,
    pullNumber,
    headSha,
    runGit = executeGit,
    now = Date.now,
  } = {}) {
    if (typeof cwd !== 'string' || cwd.length === 0) {
      throw new GitLookupError('checkout', 'working directory is missing');
    }
    if (typeof runGit !== 'function' || typeof now !== 'function') {
      throw new GitLookupError('process', 'invalid process boundary');
    }
    this.cwd = cwd;
    this.trustedSha = requireSha(trustedSha, 'trusted checkout');
    this.pullNumber = requirePullNumber(pullNumber);
    this.headSha = requireSha(headSha, 'pull request head');
    this.runGit = runGit;
    this.now = now;
    this.deadline = now() + MAX_GIT_LOOKUP_MS;
    this.fetchPromise = undefined;
    this.lookups = new Map();
  }

  async command(args, stage, maxOutputBytes = MAX_COMMAND_OUTPUT_BYTES) {
    const remainingMs = this.deadline - this.now();
    if (remainingMs <= 0) {
      throw new GitLookupError(stage, 'total time limit exceeded');
    }
    let result;
    try {
      result = await this.runGit(args, {
        cwd: this.cwd,
        maxOutputBytes,
        stage,
        timeoutMs: Math.min(MAX_GIT_COMMAND_MS, remainingMs),
      });
    } catch (error) {
      if (error instanceof GitLookupError) {
        throw error;
      }
      throw new GitLookupError(stage, processFailureDetail(error));
    }
    if (
      !result
      || !Number.isInteger(result.code)
      || !Buffer.isBuffer(result.stdout)
      || !Buffer.isBuffer(result.stderr)
    ) {
      throw new GitLookupError(stage, 'invalid process result');
    }
    if (result.stdout.length > maxOutputBytes || result.stderr.length > maxOutputBytes) {
      throw new GitLookupError(stage, 'output limit exceeded');
    }
    return result;
  }

  async readCommit(revision, stage) {
    const result = await this.command(['rev-parse', '--verify', `${revision}^{commit}`], stage, 1024);
    if (result.code !== 0 || result.stderr.length > 0) {
      throw new GitLookupError(stage, 'commit could not be resolved');
    }
    let commit;
    try {
      commit = utf8Decoder.decode(result.stdout).trim();
    } catch {
      throw new GitLookupError(stage, 'invalid commit output');
    }
    return requireSha(commit, stage);
  }

  async ensureFetched() {
    if (!this.fetchPromise) {
      this.fetchPromise = this.fetchAndVerify();
    }
    return this.fetchPromise;
  }

  async fetchAndVerify() {
    const currentHead = await this.readCommit('HEAD', 'checkout');
    if (currentHead !== this.trustedSha) {
      throw new GitLookupError('checkout', 'trusted revision changed');
    }
    const fetchResult = await this.command([
      'fetch',
      '--quiet',
      '--no-tags',
      '--no-recurse-submodules',
      '--depth=1',
      '--filter=blob:none',
      `--negotiation-tip=${this.trustedSha}`,
      'origin',
      `refs/pull/${this.pullNumber}/head`,
    ], 'fetch', MAX_FETCH_OUTPUT_BYTES);
    if (fetchResult.code !== 0) {
      throw new GitLookupError('fetch', `git exited with status ${fetchResult.code}`);
    }
    const fetchedSha = await this.readCommit('FETCH_HEAD', 'fetch');
    if (fetchedSha !== this.headSha) {
      throw new GitLookupError('fetch', 'fetched revision does not match the API snapshot');
    }
    const finalHead = await this.readCommit('HEAD', 'checkout');
    if (finalHead !== this.trustedSha) {
      throw new GitLookupError('checkout', 'trusted revision changed');
    }
  }

  async findRequirementPaths(directory, requirementId) {
    validateSearch(directory, requirementId);
    const key = JSON.stringify([directory, requirementId]);
    if (!this.lookups.has(key)) {
      this.lookups.set(key, this.search(directory, requirementId));
    }
    return this.lookups.get(key);
  }

  async search(directory, requirementId) {
    await this.ensureFetched();
    const result = await this.command([
      'grep',
      '--no-textconv',
      '--no-recurse-submodules',
      '--threads=1',
      '--full-name',
      '-z',
      '-l',
      '-F',
      '-e',
      requirementId,
      this.headSha,
      '--',
      `:(glob)${directory}/**/*.md`,
    ], 'search');
    if (result.code === 1 && result.stdout.length === 0 && result.stderr.length === 0) {
      return [];
    }
    if (result.code !== 0 || result.stderr.length > 0) {
      throw new GitLookupError('search', `git exited with status ${result.code}`);
    }
    return parseGitGrepPaths(result.stdout, this.headSha, directory);
  }
}

module.exports = { GitHeadReader };
