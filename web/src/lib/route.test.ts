import { describe, expect, it } from 'vitest';
import { backgroundView, closeEditor, formatRoute, parseRoute, withContext, type Route } from './route';

describe('parseRoute', () => {
  it.each<[string, Route]>([
    ['', { name: 'files' }],
    ['#', { name: 'files' }],
    ['#/', { name: 'files' }],
    ['#/files', { name: 'files' }],
    ['#/graph', { name: 'graph' }],
    ['#/findings', { name: 'findings' }],
    ['#/nonsense', { name: 'files' }],
    ['#/graph?ctx=claude:/home/alex/code/infra', { name: 'graph', ctx: 'claude:/home/alex/code/infra' }],
    ['#/file?id=/home/alex/code/AGENTS.md', { name: 'file', id: '/home/alex/code/AGENTS.md' }],
    [
      '#/file?id=%2Fhome%2Falex%2Fcode%2FAGENTS.md&line=12',
      { name: 'file', id: '/home/alex/code/AGENTS.md', line: 12 },
    ],
    [
      '#/file?id=/etc/claude-code/managed-settings.json%23claudeMd&line=8&view=graph',
      { name: 'file', id: '/etc/claude-code/managed-settings.json#claudeMd', line: 8, view: 'graph' },
    ],
    ['#/compare?finding=dup-1', { name: 'compare', finding: 'dup-1' }],
    [
      '#/compare?finding=dup-1&ctx=codex:/home/alex/code/infra',
      { name: 'compare', finding: 'dup-1', ctx: 'codex:/home/alex/code/infra' },
    ],
  ])('%s', (hash, want) => {
    expect(parseRoute(hash)).toEqual(want);
  });

  it('falls back to files when a required parameter is missing', () => {
    expect(parseRoute('#/file')).toEqual({ name: 'files' });
    expect(parseRoute('#/compare?ctx=x:y')).toEqual({ name: 'files', ctx: 'x:y' });
  });

  it('drops a line that is not a positive whole number', () => {
    expect(parseRoute('#/file?id=a&line=0')).toEqual({ name: 'file', id: 'a' });
    expect(parseRoute('#/file?id=a&line=-3')).toEqual({ name: 'file', id: 'a' });
    expect(parseRoute('#/file?id=a&line=4.5')).toEqual({ name: 'file', id: 'a' });
    expect(parseRoute('#/file?id=a&line=abc')).toEqual({ name: 'file', id: 'a' });
  });

  it('ignores an unknown background view and keeps malformed escapes literally', () => {
    expect(parseRoute('#/file?id=a&view=nope')).toEqual({ name: 'file', id: 'a' });
    expect(parseRoute('#/file?id=100%25&ctx=%E0%A4%A')).toEqual({ name: 'file', id: '100%', ctx: '%E0%A4%A' });
  });
});

describe('formatRoute', () => {
  it('keeps slashes and colons readable and escapes the rest', () => {
    expect(
      formatRoute({ name: 'file', id: '/etc/codex/requirements.toml#additional_developer_instructions', line: 3 }),
    ).toBe('#/file?id=/etc/codex/requirements.toml%23additional_developer_instructions&line=3');
    expect(formatRoute({ name: 'files', ctx: 'claude:/home/alex/code/my app' })).toBe(
      '#/files?ctx=claude:/home/alex/code/my%20app',
    );
    expect(formatRoute({ name: 'graph' })).toBe('#/graph');
  });

  it('round-trips every route shape, including awkward characters', () => {
    const awkward = ['/a b/c&d=e?f#g', '/x/+plus/%percent', '/home/alex/Ünïcode/AGENTS.md', 'claude:/'];
    const routes: Route[] = [];
    for (const s of awkward) {
      routes.push({ name: 'files', ctx: s });
      routes.push({ name: 'file', id: s, line: 7, view: 'findings', ctx: s });
      routes.push({ name: 'compare', finding: s, view: 'graph' });
    }
    for (const r of routes) {
      expect(parseRoute(formatRoute(r))).toEqual(r);
    }
  });
});

describe('route helpers', () => {
  it('works out which view sits behind the editor', () => {
    expect(backgroundView({ name: 'graph' })).toBe('graph');
    expect(backgroundView({ name: 'file', id: 'a' })).toBe('files');
    expect(backgroundView({ name: 'file', id: 'a', view: 'graph' })).toBe('graph');
    expect(backgroundView({ name: 'compare', finding: 'f' })).toBe('findings');
  });

  it('sets and clears the context on any route', () => {
    expect(withContext({ name: 'file', id: 'a', line: 2 }, 'c:/x')).toEqual({ name: 'file', id: 'a', line: 2, ctx: 'c:/x' });
    expect(withContext({ name: 'graph', ctx: 'c:/x' }, undefined)).toEqual({ name: 'graph' });
  });

  it('closes the editor back to its view, keeping the context', () => {
    expect(closeEditor({ name: 'file', id: 'a', view: 'graph', ctx: 'c:/x' })).toEqual({ name: 'graph', ctx: 'c:/x' });
    expect(closeEditor({ name: 'compare', finding: 'f' })).toEqual({ name: 'findings' });
    expect(closeEditor({ name: 'files' })).toEqual({ name: 'files' });
  });
});
