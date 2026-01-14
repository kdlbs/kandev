import { describe, expect, it } from 'vitest';
import {
  extractRepoName,
  formatUserHomePath,
  getRepositoryDisplayName,
  selectPreferredBranch,
  truncateRepoPath,
} from './utils';

describe('formatUserHomePath', () => {
  it('replaces mac home path with tilde', () => {
    expect(formatUserHomePath('/Users/alex/Projects/App')).toBe('~/Projects/App');
  });

  it('replaces linux home path with tilde', () => {
    expect(formatUserHomePath('/home/alex/projects/app')).toBe('~/projects/app');
  });

  it('replaces windows home path with tilde', () => {
    expect(formatUserHomePath('C:\\Users\\alex\\Projects\\App')).toBe('~/Projects/App');
  });

  it('leaves non-home paths unchanged', () => {
    expect(formatUserHomePath('/var/tmp/project')).toBe('/var/tmp/project');
  });
});

describe('truncateRepoPath', () => {
  it('returns the path when under the limit', () => {
    expect(truncateRepoPath('~/Projects/App', 40)).toBe('~/Projects/App');
  });

  it('prefers last segments for long paths', () => {
    const path = '/Users/alex/Projects/Group/RepoName';
    expect(truncateRepoPath(path, 22)).toBe('~/.../Group/RepoName');
  });

  it('falls back to last segment when space is tight', () => {
    const path = '/Users/alex/Projects/Group/RepoName';
    expect(truncateRepoPath(path, 10)).toBe('~/.../Name');
  });
});

describe('selectPreferredBranch', () => {
  it('selects origin/main first', () => {
    const branches = [
      { name: 'main', type: 'local' },
      { name: 'main', type: 'remote', remote: 'origin' },
    ];
    expect(selectPreferredBranch(branches)).toBe('origin/main');
  });

  it('falls back to origin/master', () => {
    const branches = [
      { name: 'master', type: 'remote', remote: 'origin' },
      { name: 'main', type: 'local' },
    ];
    expect(selectPreferredBranch(branches)).toBe('origin/master');
  });

  it('falls back to local main', () => {
    const branches = [{ name: 'main', type: 'local' }];
    expect(selectPreferredBranch(branches)).toBe('main');
  });

  it('returns null when no preferred branches exist', () => {
    const branches = [{ name: 'develop', type: 'local' }];
    expect(selectPreferredBranch(branches)).toBeNull();
  });
});

describe('extractRepoName', () => {
  it('extracts org/name from ssh urls', () => {
    expect(extractRepoName('git@gitlab.com:org/repo.git')).toBe('org/repo');
  });

  it('extracts org/name from https urls', () => {
    expect(extractRepoName('https://bitbucket.org/org/repo')).toBe('org/repo');
  });

  it('returns null for local paths', () => {
    expect(extractRepoName('/Users/alex/Projects/App')).toBeNull();
  });
});

describe('getRepositoryDisplayName', () => {
  it('returns a tilde path for local repositories', () => {
    expect(getRepositoryDisplayName('/Users/alex/Projects/App')).toBe('~/Projects/App');
  });

  it('returns org/name for remote repositories', () => {
    expect(getRepositoryDisplayName('https://github.com/org/repo.git')).toBe('org/repo');
  });
});
