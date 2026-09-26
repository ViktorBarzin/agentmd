import { describe, expect, it } from 'vitest';
import { ApiError, createApi } from './api';

interface Call {
  url: string;
  method: string;
  headers: Record<string, string>;
  body: string | undefined;
}

function fakeFetch(status: number, body: unknown, raw?: string) {
  const calls: Call[] = [];
  const fn = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const headers: Record<string, string> = {};
    new Headers(init?.headers).forEach((v, k) => {
      headers[k] = v;
    });
    calls.push({
      url: String(input),
      method: init?.method ?? 'GET',
      headers,
      body: typeof init?.body === 'string' ? init.body : undefined,
    });
    const text = raw ?? JSON.stringify(body);
    return new Response(text, { status, headers: { 'Content-Type': 'application/json' } });
  };
  return { fn, calls };
}

describe('api client', () => {
  it('sends the request header on reads too, with no body', async () => {
    const f = fakeFetch(200, { file: { id: 'x' }, content: 'hi' });
    const api = createApi(f.fn, 'api/');
    const res = await api.file('/etc/claude-code/managed-settings.json#claudeMd');
    await api.state();
    await api.git('/r/AGENTS.md');
    await api.job('j1');
    expect(res.content).toBe('hi');
    expect(f.calls[0].url).toBe('api/file?id=%2Fetc%2Fclaude-code%2Fmanaged-settings.json%23claudeMd');
    for (const c of f.calls) {
      expect(c.method).toBe('GET');
      expect(c.headers['x-agentmd-request']).toBe('1');
      expect(c.headers['content-type']).toBeUndefined();
      expect(c.body).toBeUndefined();
    }
  });

  it('sends every change with the request header and a JSON body', async () => {
    const f = fakeFetch(200, { repo: '/r', commit: 'abc', output: '' });
    const api = createApi(f.fn, 'api/');
    await api.commit({ ids: ['/r/AGENTS.md'], message: 'Tidy' });
    await api.scan();
    await api.probe([]);
    await api.save({ id: '/r/AGENTS.md', content: 'x', baseHash: 'h' });
    await api.push({ id: '/r/AGENTS.md' });
    await api.analyse('claude:/r');
    await api.fix('f1');
    await api.apply({ edits: [] });
    expect(f.calls.map((c) => [c.method, c.url])).toEqual([
      ['POST', 'api/commit'],
      ['POST', 'api/scan'],
      ['POST', 'api/probe'],
      ['PUT', 'api/file'],
      ['POST', 'api/push'],
      ['POST', 'api/analyse'],
      ['POST', 'api/fix'],
      ['POST', 'api/apply'],
    ]);
    for (const c of f.calls) {
      expect(c.headers['x-agentmd-request']).toBe('1');
      expect(c.headers['content-type']).toBe('application/json');
      expect(() => JSON.parse(c.body ?? '')).not.toThrow();
    }
    expect(JSON.parse(f.calls[1].body ?? '')).toEqual({});
    expect(JSON.parse(f.calls[2].body ?? '')).toEqual({ contexts: [] });
    expect(JSON.parse(f.calls[4].body ?? '')).toEqual({ id: '/r/AGENTS.md' });
  });

  it('reports a refused push with git text and no conflict body', async () => {
    const refusal = ' ! [rejected]        main -> main (non-fast-forward)\nerror: failed to push some refs';
    const f = fakeFetch(409, { error: refusal });
    const err = await createApi(f.fn, 'api/').push({ id: '/r/AGENTS.md' }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(409);
    expect((err as ApiError).message).toBe(refusal);
    expect((err as ApiError).conflict).toBeUndefined();
  });

  it('throws the server error text on a non-2xx answer', async () => {
    const f = fakeFetch(403, { error: 'This file is read-only' });
    const api = createApi(f.fn, 'api/');
    const err = await api.save({ id: 'a', content: '', baseHash: '' }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).message).toBe('This file is read-only');
    expect((err as ApiError).status).toBe(403);
    expect((err as ApiError).conflict).toBeUndefined();
  });

  it('carries the conflict body on a 409', async () => {
    const f = fakeFetch(409, { error: 'changed on disk', current: 'new text', hash: 'h2' });
    const api = createApi(f.fn, 'api/');
    const err = await api.save({ id: 'a', content: '', baseHash: 'h1' }).catch((e: unknown) => e);
    expect((err as ApiError).conflict).toEqual({ error: 'changed on disk', current: 'new text', hash: 'h2' });
  });

  it('falls back to the status when the answer is not JSON', async () => {
    const f = fakeFetch(502, null, '<html>Bad gateway</html>');
    const api = createApi(f.fn, 'api/');
    const err = await api.state().catch((e: unknown) => e);
    expect((err as ApiError).message).toBe('The server answered 502.');
  });

  it('turns a network failure into an ApiError', async () => {
    const api = createApi(async () => {
      throw new TypeError('Failed to fetch');
    }, 'api/');
    const err = await api.state().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(0);
    expect((err as ApiError).message).toContain('Could not reach agentmd');
  });

  it('encodes job ids in the path', async () => {
    const f = fakeFetch(200, { id: 'a/b', kind: 'fix', status: 'running', startedAt: '' });
    await createApi(f.fn, 'api/').job('a/b');
    expect(f.calls[0].url).toBe('api/jobs/a%2Fb');
  });
});
