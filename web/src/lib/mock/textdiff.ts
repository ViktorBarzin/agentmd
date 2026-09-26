// A small line diff for the mock, producing `git diff` style output.

function splitLines(s: string): string[] {
  if (s === '') return [];
  const lines = s.split('\n');
  if (lines[lines.length - 1] === '') lines.pop();
  return lines;
}

type Op = { kind: ' ' | '-' | '+'; text: string; a: number; b: number };

function diffOps(a: string[], b: string[]): Op[] {
  // Trim the common prefix and suffix, then run an LCS table on the middle.
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  let endA = a.length;
  let endB = b.length;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--;
    endB--;
  }
  const n = endA - start;
  const m = endB - start;
  const table = new Uint32Array((n + 1) * (m + 1));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      table[i * (m + 1) + j] =
        a[start + i] === b[start + j]
          ? table[(i + 1) * (m + 1) + j + 1] + 1
          : Math.max(table[(i + 1) * (m + 1) + j], table[i * (m + 1) + j + 1]);
    }
  }
  const ops: Op[] = [];
  for (let k = 0; k < start; k++) ops.push({ kind: ' ', text: a[k], a: k, b: k });
  let i = 0;
  let j = 0;
  while (i < n || j < m) {
    if (i < n && j < m && a[start + i] === b[start + j]) {
      ops.push({ kind: ' ', text: a[start + i], a: start + i, b: start + j });
      i++;
      j++;
    } else if (i < n && (j === m || table[(i + 1) * (m + 1) + j] >= table[i * (m + 1) + j + 1])) {
      // Removals before additions, the way git prints a changed line.
      ops.push({ kind: '-', text: a[start + i], a: start + i, b: start + j });
      i++;
    } else {
      ops.push({ kind: '+', text: b[start + j], a: start + i, b: start + j });
      j++;
    }
  }
  for (let k = 0; k < a.length - endA; k++) {
    ops.push({ kind: ' ', text: a[endA + k], a: endA + k, b: endB + k });
  }
  return ops;
}

/** A unified diff of two texts with three lines of context, or "" when they are equal. */
export function unifiedDiff(before: string, after: string, path: string, hashes: [string, string]): string {
  if (before === after) return '';
  const ops = diffOps(splitLines(before), splitLines(after));
  const context = 3;
  const ranges: Array<[number, number]> = [];
  ops.forEach((op, idx) => {
    if (op.kind === ' ') return;
    const start = Math.max(0, idx - context);
    const end = Math.min(ops.length - 1, idx + context);
    const last = ranges[ranges.length - 1];
    if (last && start <= last[1] + 1) last[1] = Math.max(last[1], end);
    else ranges.push([start, end]);
  });
  const out = [
    `diff --git a/${path} b/${path}`,
    `index ${hashes[0].slice(0, 7)}..${hashes[1].slice(0, 7)} 100644`,
    `--- a/${path}`,
    `+++ b/${path}`,
  ];
  for (const [start, end] of ranges) {
    const hunk = ops.slice(start, end + 1);
    const oldLines = hunk.filter((o) => o.kind !== '+');
    const newLines = hunk.filter((o) => o.kind !== '-');
    const oldStart = oldLines.length ? oldLines[0].a + 1 : hunk[0].a;
    const newStart = newLines.length ? newLines[0].b + 1 : hunk[0].b;
    out.push(`@@ -${oldStart},${oldLines.length} +${newStart},${newLines.length} @@`);
    for (const o of hunk) out.push(o.kind + o.text);
  }
  return out.join('\n') + '\n';
}

