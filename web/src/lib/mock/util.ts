// Helpers the mock uses to describe file contents the way the backend does.

const encoder = new TextEncoder();

export function byteLength(s: string): number {
  return encoder.encode(s).length;
}

/** Lines in a file; a trailing newline does not start a new line. */
export function countLines(s: string): number {
  if (s === '') return 0;
  const n = s.split('\n').length;
  return s.endsWith('\n') ? n - 1 : n;
}

function fnv1a(s: string, seed: number): number {
  let h = seed >>> 0;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h >>> 0;
}

/** A stable 16-hex-digit content hash. The real backend uses a stronger one. */
export function hashText(s: string): string {
  return fnv1a(s, 0x811c9dc5).toString(16).padStart(8, '0') + fnv1a(s, 0x01234567).toString(16).padStart(8, '0');
}

/** A fake 40-digit commit id. */
export function fakeSha(seed: string): string {
  let out = '';
  for (let i = 0; out.length < 40; i++) out += fnv1a(seed + i, 0x9e3779b9).toString(16).padStart(8, '0');
  return out.slice(0, 40);
}
