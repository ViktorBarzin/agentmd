import { describe, expect, it } from 'vitest';
import { classifyDiffLine, diffStats, parseDiff } from './diff';

describe('classifyDiffLine', () => {
  it.each([
    ['diff --git a/AGENTS.md b/AGENTS.md', 'meta'],
    ['index 1a2b3c4..5d6e7f8 100644', 'meta'],
    ['--- a/AGENTS.md', 'meta'],
    ['+++ b/AGENTS.md', 'meta'],
    ['new file mode 100644', 'meta'],
    ['@@ -1,3 +1,4 @@ heading', 'hunk'],
    ['+added', 'add'],
    ['-removed', 'del'],
    [' context', 'context'],
    ['', 'context'],
    ['\\ No newline at end of file', 'note'],
  ] as const)('%s is %s', (line, kind) => {
    expect(classifyDiffLine(line)).toBe(kind);
  });
});

const sample = [
  'diff --git a/AGENTS.md b/AGENTS.md',
  'index 1a2b3c4..5d6e7f8 100644',
  '--- a/AGENTS.md',
  '+++ b/AGENTS.md',
  '@@ -118,4 +118,3 @@ ## Git',
  ' keep one',
  '--- a removed line that starts with two dashes',
  '-another removed line',
  '+++ an added line that starts with two pluses',
  ' keep two',
  '\\ No newline at end of file',
].join('\n');

describe('parseDiff', () => {
  it('uses the hunk counts, so lines that look like headers stay in the hunk', () => {
    const lines = parseDiff(sample);
    expect(lines.map((l) => l.kind)).toEqual([
      'meta',
      'meta',
      'meta',
      'meta',
      'hunk',
      'context',
      'del',
      'del',
      'add',
      'context',
      'note',
    ]);
  });

  it('numbers old and new lines from the hunk header', () => {
    const lines = parseDiff(sample);
    expect(lines[5]).toMatchObject({ kind: 'context', oldLine: 118, newLine: 118 });
    expect(lines[6]).toMatchObject({ kind: 'del', oldLine: 119 });
    expect(lines[6].newLine).toBeUndefined();
    expect(lines[7]).toMatchObject({ kind: 'del', oldLine: 120 });
    expect(lines[8]).toMatchObject({ kind: 'add', newLine: 119 });
    expect(lines[9]).toMatchObject({ kind: 'context', oldLine: 121, newLine: 120 });
  });

  it('handles hunk headers without counts and several files', () => {
    const text = [
      'diff --git a/a.md b/a.md',
      '--- a/a.md',
      '+++ b/a.md',
      '@@ -3 +3 @@',
      '-old',
      '+new',
      'diff --git a/b.md b/b.md',
      '--- a/b.md',
      '+++ b/b.md',
      '@@ -1,0 +1,1 @@',
      '+first',
      '',
    ].join('\n');
    const kinds = parseDiff(text).map((l) => l.kind);
    expect(kinds).toEqual(['meta', 'meta', 'meta', 'hunk', 'del', 'add', 'meta', 'meta', 'meta', 'hunk', 'add']);
  });

  it('returns nothing for an empty diff', () => {
    expect(parseDiff('')).toEqual([]);
  });

  it('counts additions and deletions', () => {
    expect(diffStats(parseDiff(sample))).toEqual({ added: 1, removed: 2 });
  });
});
