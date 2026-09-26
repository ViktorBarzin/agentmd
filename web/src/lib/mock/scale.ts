// Grows the fixture to a few hundred files and references, for checking
// that the UI stays smooth at the size of a real workstation. Opt in with
// ?mockScale=<projects> in the mock's URL.
import type { AgentFile, Ref, RuntimeContext } from '../types';
import { describe, P, shorten, type Fixture } from './fixture';

export function scaleFixture(fx: Fixture, projects: number): void {
  const { state, contents, links } = fx;
  const home = state.contexts.find((c) => c.harness === 'claude' && c.dir === state.home);
  const webapp = state.contexts.find((c) => c.harness === 'claude' && c.dir.endsWith('/webapp'));
  const [org, user] = home?.entries ?? [];
  const code = webapp?.entries.find((e) => e.fileId === P.codeClaude);
  if (!org || !user || !code) return;

  const file = (id: string, extra: Partial<AgentFile>, content: string): AgentFile =>
    describe(
      {
        id,
        path: id,
        realPath: id,
        display: shorten(id),
        kind: 'instruction',
        scope: 'project',
        harnesses: [],
        size: 0,
        lines: 0,
        hash: '',
        access: { writable: true },
        ...extra,
      },
      content,
    );

  let previous = '';
  for (let i = 0; i < projects; i++) {
    const name = `p${String(i).padStart(3, '0')}`;
    const repo = `/home/alex/code/lab/${name}`;
    const agents = `${repo}/AGENTS.md`;
    const claude = `${repo}/CLAUDE.md`;
    const notes = `${repo}/docs/notes.md`;
    const skill = `${repo}/.agents/skills/${name}-release/SKILL.md`;
    const agentsText = [
      `# ${name}`,
      '',
      `Project ${name} in the lab. It follows the shared rules in ~/code/AGENTS.md.`,
      '',
      '- The design lives in docs/notes.md.',
      previous ? `- It depends on ${shorten(previous)}.` : '- It depends on nothing else in the lab.',
      '- Run `make test` before every push.',
      '',
    ].join('\n');
    const notesText = `# Notes for ${name}\n\nNothing decided yet.\n`;
    const skillText = `---\nname: ${name}-release\ndescription: Cut a release of ${name}.\n---\n\nTag, build and publish ${name}.\n`;
    contents.set(agents, agentsText);
    contents.set(notes, notesText);
    contents.set(skill, skillText);
    links.set(claude, agents);

    state.files.push(
      file(agents, { repo, harnesses: ['claude', 'agents-md'] }, agentsText),
      file(claude, { repo, harnesses: ['claude'], isLink: true, linkTarget: 'AGENTS.md', realPath: agents }, agentsText),
      file(notes, { repo, kind: 'doc' }, notesText),
      file(skill, { repo, kind: 'skill', name: `${name}-release`, harnesses: ['codex'] }, skillText),
    );

    const refs: Ref[] = [
      { from: claude, to: agents, kind: 'symlink', sub: 'link' },
      { from: agents, to: notes, kind: 'mention', sub: 'path', line: 5, text: 'docs/notes.md' },
      { from: agents, to: P.codeAgents, kind: 'mention', sub: 'path', line: 3, text: '~/code/AGENTS.md' },
    ];
    if (previous) refs.push({ from: agents, to: previous, kind: 'mention', sub: 'path', line: 6, text: shorten(previous) });
    state.refs.push(...refs);

    const size = (id: string) => state.files.find((f) => f.id === id)?.size ?? 0;
    const probed: RuntimeContext = {
      id: `claude:${repo}`,
      harness: 'claude',
      dir: repo,
      display: shorten(repo),
      source: 'probe',
      version: home?.version,
      probedAt: home?.probedAt,
      entries: [org, user, code, { fileId: claude, label: 'Project instructions', bytes: size(claude) }],
      skills: home?.skills ?? [],
      subagents: home?.subagents ?? [],
      bytes: 0,
    };
    probed.bytes = probed.entries.reduce((n, e) => n + e.bytes, 0);
    const statik: RuntimeContext = {
      id: `agents-md:${repo}`,
      harness: 'agents-md',
      dir: repo,
      display: shorten(repo),
      source: 'static',
      entries: [{ fileId: agents, label: 'Project doc', bytes: size(agents) }],
      skills: [],
      subagents: [],
      bytes: size(agents),
    };
    state.contexts.push(probed, statik);
    previous = agents;
  }
}
