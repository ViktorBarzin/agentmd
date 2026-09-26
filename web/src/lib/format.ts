// Small formatting helpers shared by the views.

export function relativeTime(iso: string | undefined, now: number): string {
  if (!iso) return 'never';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return 'unknown';
  const seconds = Math.round((now - t) / 1000);
  if (seconds < 45) return 'just now';
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.round(hours / 24);
  if (days === 1) return 'yesterday';
  if (days < 30) return `${days} days ago`;
  return new Date(t).toISOString().slice(0, 10);
}

export function formatTime(iso: string | undefined): string {
  if (!iso) return '';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  return new Date(t).toLocaleString();
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${trimZero((n / 1024).toFixed(1))} KB`;
  return `${trimZero((n / (1024 * 1024)).toFixed(1))} MB`;
}

function trimZero(s: string): string {
  return s.endsWith('.0') ? s.slice(0, -2) : s;
}

export function formatCount(n: number): string {
  return n.toLocaleString('en-US');
}

/** The last path segment, without an embedded "#field" suffix. */
export function basename(path: string): string {
  const noField = stripField(path);
  const i = noField.lastIndexOf('/');
  return i === -1 ? noField : noField.slice(i + 1);
}

export function dirname(path: string): string {
  const noField = stripField(path);
  const i = noField.lastIndexOf('/');
  if (i === -1) return '';
  if (i === 0) return '/';
  return noField.slice(0, i);
}

function stripField(path: string): string {
  const hash = path.indexOf('#');
  return hash === -1 ? path : path.slice(0, hash);
}

/** Splits a path for display: the directory part keeps its trailing slash. */
export function splitPath(path: string): { dir: string; name: string } {
  const hash = path.indexOf('#');
  const searchIn = hash === -1 ? path : path.slice(0, hash);
  const i = searchIn.lastIndexOf('/');
  if (i === -1) return { dir: '', name: path };
  return { dir: path.slice(0, i + 1), name: path.slice(i + 1) };
}

export function shortenHome(path: string, home: string): string {
  if (!home) return path;
  if (path === home) return '~';
  if (path.startsWith(home + '/')) return '~' + path.slice(home.length);
  return path;
}

export function lineRange(start: number, end: number): string {
  return start === end ? `${start}` : `${start}-${end}`;
}

/** Ends a note with a full stop unless it already ends a sentence. */
export function sentence(text: string): string {
  const t = text.trim();
  if (!t) return '';
  return /[.!?:)]$/.test(t) ? t : t + '.';
}

export function plural(n: number, word: string, pluralWord = word + 's'): string {
  return `${formatCount(n)} ${n === 1 ? word : pluralWord}`;
}
