import { describe, expect, it } from 'vitest';
import { harnessFiles } from './harness';
import { file, ref, state } from './test-helpers';

const hub = '/home/alex/.agents/AGENTS.md';
const core = '/home/alex/.agents/core.md';
const claudeLink = '/home/alex/.claude/CLAUDE.md';
const codexOnly = '/home/alex/code/app/AGENTS.override.md';
const doc = '/home/alex/code/app/docs/style.md';
const unrelated = '/home/alex/code/other/notes.md';

const s = state({
  files: [
    file(claudeLink, { harnesses: ['claude'], isLink: true, linkTarget: hub }),
    file(hub, { harnesses: ['claude', 'codex'], access: { writable: false, builtFrom: [core] } }),
    file(core, { harnesses: [] }),
    file(codexOnly, { harnesses: ['codex'] }),
    file(doc, { kind: 'doc', harnesses: [] }),
    file(unrelated, { kind: 'doc', harnesses: [] }),
  ],
  refs: [
    ref(claudeLink, hub, 'symlink', { sub: 'link' }),
    ref(hub, core, 'build', { sub: 'header' }),
    ref(codexOnly, doc, 'mention', { sub: 'path', line: 3 }),
  ],
});

describe('harnessFiles', () => {
  it('keeps files the harness reads, plus the parts and docs they reach one hop out', () => {
    expect([...harnessFiles(s, 'claude')].sort()).toEqual([hub, core, claudeLink].sort());
    expect([...harnessFiles(s, 'codex')].sort()).toEqual([hub, core, codexOnly, doc].sort());
  });

  it('is empty for a harness nothing reads', () => {
    expect(harnessFiles(s, 'pi').size).toBe(0);
  });
});
