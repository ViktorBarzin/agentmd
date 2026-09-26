import { markdownLanguage } from '@codemirror/lang-markdown';
import { Text } from '@codemirror/state';
import { MarkdownParser } from '@lezer/markdown';
import { describe, expect, it } from 'vitest';
import { clampSpan, findingDiagnostics, frontmatter } from './cm';
import { finding } from './test-helpers';

function topNodes(doc: string): Array<[string, number, number]> {
  const base = markdownLanguage.parser;
  if (!(base instanceof MarkdownParser)) throw new Error('lang-markdown no longer uses a MarkdownParser');
  const tree = base.configure([frontmatter]).parse(doc);
  const out: Array<[string, number, number]> = [];
  for (let n = tree.topNode.firstChild; n; n = n.nextSibling) out.push([n.name, n.from, n.to]);
  return out;
}

describe('frontmatter', () => {
  it('parses a frontmatter block instead of a heading underline', () => {
    const doc = '---\nname: tdd\ndescription: Test first.\n---\n\n# Tests\n';
    const nodes = topNodes(doc);
    expect(nodes[0]).toEqual(['Frontmatter', 0, doc.indexOf('---\n\n') + 3]);
    expect(nodes.map((n) => n[0])).toEqual(['Frontmatter', 'ATXHeading1']);
  });

  it('only counts a block at the very start of the file', () => {
    const doc = '# Title\n\nText\n\n---\n\nMore\n';
    expect(topNodes(doc).map((n) => n[0])).toEqual(['ATXHeading1', 'Paragraph', 'HorizontalRule', 'Paragraph']);
  });
});

const doc = Text.of(['# Title', '', 'one', 'two', 'three']);

describe('clampSpan', () => {
  it('keeps spans inside the document', () => {
    expect(clampSpan(doc, { start: 3, end: 4 })).toEqual({ start: 3, end: 4 });
    expect(clampSpan(doc, { start: 0, end: 99 })).toEqual({ start: 1, end: 5 });
    expect(clampSpan(doc, { start: 40, end: 50 })).toEqual({ start: 5, end: 5 });
    expect(clampSpan(doc, { start: 4, end: 2 })).toEqual({ start: 4, end: 4 });
  });
});

describe('findingDiagnostics', () => {
  it('turns each span in the file into a diagnostic covering whole lines', () => {
    const findings = [
      finding('p', {
        severity: 'problem',
        kind: 'duplicate',
        summary: 'Same lines',
        spans: [
          { fileId: 'a', startLine: 3, endLine: 4 },
          { fileId: 'b', startLine: 1, endLine: 1 },
        ],
      }),
      finding('h', { severity: 'hint', kind: 'dangling', summary: 'Missing file', spans: [{ fileId: 'a', startLine: 1, endLine: 1 }] }),
    ];
    const got = findingDiagnostics(doc, findings, 'a');
    expect(got).toEqual([
      { from: 0, to: 7, severity: 'warning', message: 'Dangling reference: Missing file', source: 'agentmd' },
      { from: 9, to: 16, severity: 'error', message: 'Duplicate: Same lines', source: 'agentmd' },
    ]);
  });

  it('clamps spans that run past the end after an edit', () => {
    const f = finding('x', { spans: [{ fileId: 'a', startLine: 5, endLine: 9 }] });
    expect(findingDiagnostics(doc, [f], 'a')[0]).toMatchObject({ from: doc.line(5).from, to: doc.length });
  });
});
