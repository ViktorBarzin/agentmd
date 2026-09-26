// Groups files for the Files view: org policy, user, skills by source, then
// one group per repository.
import { shortenHome } from './format';
import type { AgentFile } from './types';

export interface FileGroup {
  id: string;
  label: string;
  /** Set for repository groups: rows show paths relative to it. */
  root?: string;
  files: AgentFile[];
}

function skillSource(f: AgentFile): { id: string; label: string; rank: number } {
  if (f.scope === 'plugin') return { id: 'skills:plugin', label: 'Plugin skills', rank: 2 };
  const source = f.access.managed?.source;
  if (source) return { id: 'skills:' + source, label: 'Skills from ' + source, rank: 1 };
  return { id: 'skills:own', label: 'Your skills', rank: 0 };
}

function depth(path: string): number {
  return path.split('/').length;
}

export function groupFiles(files: AgentFile[], home: string, order?: Map<string, number>): FileGroup[] {
  const org: FileGroup = { id: 'org', label: 'Org policy', files: [] };
  const user: FileGroup = { id: 'user', label: 'User', files: [] };
  const other: FileGroup = { id: 'other', label: 'Other', files: [] };
  const skills = new Map<string, FileGroup & { rank: number }>();
  const repos = new Map<string, FileGroup>();

  for (const f of files) {
    if (f.repo) {
      let g = repos.get(f.repo);
      if (!g) {
        g = { id: 'repo:' + f.repo, label: shortenHome(f.repo, home), root: f.repo, files: [] };
        repos.set(f.repo, g);
      }
      g.files.push(f);
    } else if (f.scope === 'org') {
      org.files.push(f);
    } else if (f.kind === 'skill' && (f.scope === 'user' || f.scope === 'plugin')) {
      const s = skillSource(f);
      let g = skills.get(s.id);
      if (!g) {
        g = { id: s.id, label: s.label, rank: s.rank, files: [] };
        skills.set(s.id, g);
      }
      g.files.push(f);
    } else if (f.scope === 'user') {
      user.files.push(f);
    } else {
      other.files.push(f);
    }
  }

  const byPath = (root: string | undefined) => (a: AgentFile, b: AgentFile) => {
    const pa = order?.get(a.id);
    const pb = order?.get(b.id);
    if (pa !== undefined || pb !== undefined) {
      if (pa === undefined) return 1;
      if (pb === undefined) return -1;
      if (pa !== pb) return pa - pb;
    }
    const ra = root ? a.id.slice(root.length) : a.id;
    const rb = root ? b.id.slice(root.length) : b.id;
    const d = depth(ra) - depth(rb);
    if (d !== 0) return d;
    return ra.localeCompare(rb);
  };

  const skillGroups = [...skills.values()].sort((a, b) => a.rank - b.rank || a.label.localeCompare(b.label));
  const repoGroups = [...repos.values()].sort((a, b) => (a.root ?? '').localeCompare(b.root ?? ''));
  const all: FileGroup[] = [
    org,
    user,
    ...skillGroups.map(({ id, label, files }) => ({ id, label, files })),
    ...repoGroups,
    other,
  ];
  for (const g of all) g.files.sort(byPath(g.root));
  return all.filter((g) => g.files.length > 0);
}

/** The text a row shows: relative to its repository, or the display path. */
export function rowLabel(f: AgentFile, group: FileGroup | undefined, home: string): string {
  if (group?.root && f.id.startsWith(group.root + '/')) return f.id.slice(group.root.length + 1);
  return f.display || shortenHome(f.id, home);
}

export function matchesQuery(f: AgentFile, query: string): boolean {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return true;
  const hay = [f.display, f.id, f.name ?? '', f.description ?? '', f.kind].join('\n').toLowerCase();
  return terms.every((t) => hay.includes(t));
}
