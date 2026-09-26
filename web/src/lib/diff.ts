// Reading `git diff` output for the changes panel.

export type DiffLineKind = 'meta' | 'hunk' | 'add' | 'del' | 'context' | 'note';

export interface DiffLine {
  kind: DiffLineKind;
  text: string;
  oldLine?: number;
  newLine?: number;
}

const META = /^(diff |index |--- |\+\+\+ |new file mode|deleted file mode|old mode|new mode|similarity index|dissimilarity index|rename from|rename to|copy from|copy to|Binary files)/;

/** Classifies one line on its own, without knowing where the hunks are. */
export function classifyDiffLine(line: string): DiffLineKind {
  if (line.startsWith('@@')) return 'hunk';
  if (META.test(line)) return 'meta';
  if (line.startsWith('\\')) return 'note';
  if (line.startsWith('+')) return 'add';
  if (line.startsWith('-')) return 'del';
  return 'context';
}

const HUNK = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/;

/**
 * Parses a unified diff. Inside a hunk, lines are read by the hunk's line
 * counts, so a removed line that starts with "--" is not mistaken for a
 * file header.
 */
export function parseDiff(diff: string): DiffLine[] {
  if (!diff) return [];
  const raw = diff.split('\n');
  if (raw[raw.length - 1] === '') raw.pop();
  const out: DiffLine[] = [];
  let oldLeft = 0;
  let newLeft = 0;
  let oldNo = 0;
  let newNo = 0;
  for (const text of raw) {
    if (oldLeft > 0 || newLeft > 0) {
      const c = text.charAt(0);
      if (c === '\\') {
        out.push({ kind: 'note', text });
      } else if (c === '-') {
        out.push({ kind: 'del', text, oldLine: oldNo++ });
        oldLeft--;
      } else if (c === '+') {
        out.push({ kind: 'add', text, newLine: newNo++ });
        newLeft--;
      } else {
        out.push({ kind: 'context', text, oldLine: oldNo++, newLine: newNo++ });
        oldLeft--;
        newLeft--;
      }
      continue;
    }
    const m = HUNK.exec(text);
    if (m) {
      oldNo = Number(m[1]);
      oldLeft = m[2] === undefined ? 1 : Number(m[2]);
      newNo = Number(m[3]);
      newLeft = m[4] === undefined ? 1 : Number(m[4]);
      out.push({ kind: 'hunk', text });
      continue;
    }
    const kind = classifyDiffLine(text);
    out.push({ kind: kind === 'context' || kind === 'add' || kind === 'del' ? 'meta' : kind, text });
  }
  return out;
}

export function diffStats(lines: DiffLine[]): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  for (const l of lines) {
    if (l.kind === 'add') added++;
    else if (l.kind === 'del') removed++;
  }
  return { added, removed };
}
