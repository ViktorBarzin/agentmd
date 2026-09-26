// Typed client for the agentmd HTTP API (docs/api.md).
import type {
  ApiError as ApiErrorBody,
  ApplyRequest,
  ApplyResponse,
  CommitRequest,
  CommitResponse,
  Conflict,
  FileResponse,
  GitResponse,
  Job,
  PushRequest,
  PushResponse,
  SaveRequest,
  SaveResponse,
  State,
} from './types';

export class ApiError extends Error {
  readonly status: number;
  /** Set when a save answered 409 because the file changed on disk. */
  readonly conflict?: Conflict;

  constructor(message: string, status: number, conflict?: Conflict) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.conflict = conflict;
  }
}

function isRecord(x: unknown): x is Record<string, unknown> {
  return typeof x === 'object' && x !== null;
}

function isErrorBody(x: unknown): x is ApiErrorBody {
  return isRecord(x) && typeof x.error === 'string';
}

function isConflict(x: unknown): x is Conflict {
  return isErrorBody(x) && isRecord(x) && typeof x.current === 'string' && typeof x.hash === 'string';
}

export interface Api {
  state(): Promise<State>;
  scan(): Promise<State>;
  file(id: string): Promise<FileResponse>;
  save(req: SaveRequest): Promise<SaveResponse>;
  git(id: string): Promise<GitResponse>;
  commit(req: CommitRequest): Promise<CommitResponse>;
  push(req: PushRequest): Promise<PushResponse>;
  probe(contexts: string[]): Promise<State>;
  analyse(context: string): Promise<Job>;
  fix(findingId: string): Promise<Job>;
  job(id: string): Promise<Job>;
  apply(req: ApplyRequest): Promise<ApplyResponse>;
}

type FetchFn = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

/**
 * Creates a client. `base` is relative so the UI works behind a proxy that
 * serves agentmd under a sub-path; the global fetch is looked up per call so
 * the mock can replace it.
 */
export function createApi(fetchFn: FetchFn = (input, init) => globalThis.fetch(input, init), base = 'api/'): Api {
  async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
    // Every /api request carries the header, reads included: the server
    // refuses requests without it, so another site cannot drive agentmd.
    const headers: Record<string, string> = { Accept: 'application/json', 'X-Agentmd-Request': '1' };
    const init: RequestInit = { method, headers };
    if (method !== 'GET') {
      headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body ?? {});
    }
    let res: Response;
    try {
      res = await fetchFn(base + path, init);
    } catch (e) {
      const why = e instanceof Error ? e.message : String(e);
      throw new ApiError(`Could not reach agentmd: ${why}`, 0);
    }
    const text = await res.text();
    let parsed: unknown = null;
    if (text) {
      try {
        parsed = JSON.parse(text);
      } catch {
        parsed = null;
      }
    }
    if (!res.ok) {
      const message = isErrorBody(parsed) && parsed.error ? parsed.error : `The server answered ${res.status}.`;
      throw new ApiError(message, res.status, res.status === 409 && isConflict(parsed) ? parsed : undefined);
    }
    if (parsed === null) throw new ApiError('The server sent an answer that is not JSON.', res.status);
    return parsed as T;
  }

  const q = encodeURIComponent;
  return {
    state: () => call<State>('GET', 'state'),
    scan: () => call<State>('POST', 'scan', {}),
    file: (id) => call<FileResponse>('GET', `file?id=${q(id)}`),
    save: (req) => call<SaveResponse>('PUT', 'file', req),
    git: (id) => call<GitResponse>('GET', `git?id=${q(id)}`),
    commit: (req) => call<CommitResponse>('POST', 'commit', req),
    push: (req) => call<PushResponse>('POST', 'push', req),
    probe: (contexts) => call<State>('POST', 'probe', { contexts }),
    analyse: (context) => call<Job>('POST', 'analyse', { context }),
    fix: (findingId) => call<Job>('POST', 'fix', { findingId }),
    job: (id) => call<Job>('GET', `jobs/${q(id)}`),
    apply: (req) => call<ApplyResponse>('POST', 'apply', req),
  };
}

export const api = createApi();

export function errorMessage(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}
