import { describe, expect, it } from 'vitest';
import { indexState } from './model';
import { ctx, file, finding, ref, state } from './test-helpers';

describe('indexState', () => {
  const s = state({
    files: [file('/a'), file('/b', { isLink: true }), file('/c')],
    refs: [
      ref('/b', '/a', 'symlink'),
      ref('/a', '/c', 'mention', { line: 3 }),
      ref('/a', '/gone.md', 'mention', { dangling: true, line: 9 }),
    ],
    contexts: [ctx('claude:/x', ['/b', '/c'])],
    findings: [
      finding('f1', { severity: 'problem', spans: [{ fileId: '/a', startLine: 1, endLine: 2 }] }),
      finding('f2', { spans: [{ fileId: '/a', startLine: 5, endLine: 5 }, { fileId: '/c', startLine: 1, endLine: 1 }] }),
    ],
  });
  const ix = indexState(s);

  it('looks files, contexts and findings up by id', () => {
    expect(ix.files.get('/c')?.id).toBe('/c');
    expect(ix.contexts.get('claude:/x')?.entries).toHaveLength(2);
    expect(ix.findings.get('f2')?.spans).toHaveLength(2);
  });

  it('indexes references in both directions', () => {
    expect(ix.refsFrom.get('/a')?.map((r) => r.to)).toEqual(['/c', '/gone.md']);
    expect(ix.refsTo.get('/a')?.map((r) => r.from)).toEqual(['/b']);
    expect(ix.linksTo.get('/a')).toEqual(['/b']);
  });

  it('counts findings per file', () => {
    expect(ix.counts.get('/a')).toEqual({ problem: 1, hint: 1 });
    expect(ix.counts.get('/c')).toEqual({ problem: 0, hint: 1 });
    expect(ix.counts.get('/b')).toBeUndefined();
  });

  it('records where each file loads', () => {
    expect(ix.loadedIn.get('/c')?.map((l) => [l.context.id, l.position])).toEqual([['claude:/x', 2]]);
    expect(ix.loadedIn.get('/a')).toBeUndefined();
  });
});
