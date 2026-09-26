// Hash routes, so every view can be bookmarked and served from any path.
//
//   #/files  #/graph  #/findings
//   #/file?id=<id>&line=<n>&view=<view behind the editor>
//   #/compare?finding=<id>&view=<view>
//
// Any route may carry ctx=<context id>.

export const VIEWS = ['files', 'graph', 'findings'] as const;
export type View = (typeof VIEWS)[number];

export type ViewRoute = { name: View; ctx?: string };
export type FileRoute = { name: 'file'; id: string; line?: number; view?: View; ctx?: string };
export type CompareRoute = { name: 'compare'; finding: string; view?: View; ctx?: string };
export type Route = ViewRoute | FileRoute | CompareRoute;

function isView(s: string | undefined): s is View {
  return s !== undefined && (VIEWS as readonly string[]).includes(s);
}

function decode(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

/** Escapes a query value but keeps "/", ":" and "@" readable. */
function encode(s: string): string {
  return encodeURIComponent(s).replace(/%2F/gi, '/').replace(/%3A/gi, ':').replace(/%40/gi, '@');
}

function parseQuery(q: string): Map<string, string> {
  const out = new Map<string, string>();
  if (!q) return out;
  for (const part of q.split('&')) {
    if (!part) continue;
    const eq = part.indexOf('=');
    const key = decode(eq === -1 ? part : part.slice(0, eq));
    const value = eq === -1 ? '' : decode(part.slice(eq + 1));
    if (!out.has(key)) out.set(key, value);
  }
  return out;
}

function parseLine(s: string | undefined): number | undefined {
  if (!s || !/^\d+$/.test(s)) return undefined;
  const n = Number(s);
  return n > 0 ? n : undefined;
}

export function parseRoute(hash: string): Route {
  let h = hash.startsWith('#') ? hash.slice(1) : hash;
  if (h.startsWith('/')) h = h.slice(1);
  const qi = h.indexOf('?');
  const name = qi === -1 ? h : h.slice(0, qi);
  const q = parseQuery(qi === -1 ? '' : h.slice(qi + 1));
  const ctx = q.get('ctx') || undefined;
  const view = q.get('view');

  let route: Route;
  if (name === 'file' && q.get('id')) {
    const r: FileRoute = { name: 'file', id: q.get('id') ?? '' };
    const line = parseLine(q.get('line'));
    if (line !== undefined) r.line = line;
    if (isView(view)) r.view = view;
    route = r;
  } else if (name === 'compare' && q.get('finding')) {
    const r: CompareRoute = { name: 'compare', finding: q.get('finding') ?? '' };
    if (isView(view)) r.view = view;
    route = r;
  } else {
    route = { name: isView(name) ? name : 'files' };
  }
  if (ctx !== undefined) route.ctx = ctx;
  return route;
}

export function formatRoute(route: Route): string {
  const params: [string, string][] = [];
  if (route.name === 'file') {
    params.push(['id', route.id]);
    if (route.line !== undefined) params.push(['line', String(route.line)]);
    if (route.view) params.push(['view', route.view]);
  } else if (route.name === 'compare') {
    params.push(['finding', route.finding]);
    if (route.view) params.push(['view', route.view]);
  }
  if (route.ctx) params.push(['ctx', route.ctx]);
  const query = params.map(([k, v]) => `${k}=${encode(v)}`).join('&');
  return `#/${route.name}${query ? '?' + query : ''}`;
}

/** The view shown in the main area, behind the editor or compare mode. */
export function backgroundView(route: Route): View {
  if (route.name === 'file') return route.view ?? 'files';
  if (route.name === 'compare') return route.view ?? 'findings';
  return route.name;
}

export function withContext(route: Route, ctx: string | undefined): Route {
  const next = { ...route };
  if (ctx) next.ctx = ctx;
  else delete next.ctx;
  return next;
}

export function closeEditor(route: Route): Route {
  const r: Route = { name: backgroundView(route) };
  if (route.ctx) r.ctx = route.ctx;
  return r;
}
