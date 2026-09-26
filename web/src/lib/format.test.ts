import { describe, expect, it } from 'vitest';
import {
  basename,
  dirname,
  formatBytes,
  formatCount,
  lineRange,
  plural,
  relativeTime,
  sentence,
  shortenHome,
  splitPath,
} from './format';

const now = Date.parse('2026-09-26T12:00:00Z');

describe('relativeTime', () => {
  it.each([
    ['2026-09-26T11:59:40Z', 'just now'],
    ['2026-09-26T12:00:30Z', 'just now'],
    ['2026-09-26T11:58:00Z', '2 min ago'],
    ['2026-09-26T09:00:00Z', '3 h ago'],
    ['2026-09-25T10:00:00Z', 'yesterday'],
    ['2026-09-21T12:00:00Z', '5 days ago'],
    ['2026-06-01T12:00:00Z', '2026-06-01'],
  ])('%s is %s', (iso, want) => {
    expect(relativeTime(iso, now)).toBe(want);
  });

  it('says never for a missing time and unknown for garbage', () => {
    expect(relativeTime(undefined, now)).toBe('never');
    expect(relativeTime('soon', now)).toBe('unknown');
  });
});

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [900, '900 B'],
    [1024, '1 KB'],
    [36059, '35.2 KB'],
    [3 * 1024 * 1024, '3 MB'],
  ])('%d is %s', (n, want) => {
    expect(formatBytes(n)).toBe(want);
  });
});

describe('paths', () => {
  it('takes the base name, ignoring an embedded field', () => {
    expect(basename('/home/alex/code/AGENTS.md')).toBe('AGENTS.md');
    expect(basename('/etc/claude-code/managed-settings.json#claudeMd')).toBe('managed-settings.json');
    expect(basename('AGENTS.md')).toBe('AGENTS.md');
  });

  it('takes the directory', () => {
    expect(dirname('/home/alex/code/AGENTS.md')).toBe('/home/alex/code');
    expect(dirname('/AGENTS.md')).toBe('/');
    expect(dirname('AGENTS.md')).toBe('');
  });

  it('splits a display path into directory and name', () => {
    expect(splitPath('~/code/infra/AGENTS.md')).toEqual({ dir: '~/code/infra/', name: 'AGENTS.md' });
    expect(splitPath('AGENTS.md')).toEqual({ dir: '', name: 'AGENTS.md' });
    expect(splitPath('/etc/x/managed-settings.json#claudeMd')).toEqual({
      dir: '/etc/x/',
      name: 'managed-settings.json#claudeMd',
    });
  });

  it('shortens the home directory to a tilde', () => {
    expect(shortenHome('/home/alex/code', '/home/alex')).toBe('~/code');
    expect(shortenHome('/home/alex', '/home/alex')).toBe('~');
    expect(shortenHome('/home/alexandra/x', '/home/alex')).toBe('/home/alexandra/x');
    expect(shortenHome('/etc/codex', '/home/alex')).toBe('/etc/codex');
  });
});

describe('small words', () => {
  it('formats line ranges', () => {
    expect(lineRange(4, 4)).toBe('4');
    expect(lineRange(10, 14)).toBe('10-14');
  });

  it('pluralises', () => {
    expect(plural(1, 'file')).toBe('1 file');
    expect(plural(2, 'file')).toBe('2 files');
    expect(plural(0, 'entry', 'entries')).toBe('0 entries');
  });

  it('groups thousands', () => {
    expect(formatCount(36059)).toBe('36,059');
  });

  it('ends notes with a full stop', () => {
    expect(sentence('Refreshed by `skills update`; edits here are replaced')).toBe(
      'Refreshed by `skills update`; edits here are replaced.',
    );
    expect(sentence('Owned by root.')).toBe('Owned by root.');
    expect(sentence('  ')).toBe('');
  });
});
