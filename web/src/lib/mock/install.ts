// Replaces fetch for /api/ requests with the in-memory MockServer. Loaded by
// a dynamic import only when VITE_MOCK=1, so it never ships in a build.
import { MockServer } from './server';

export interface MockControls {
  /** Changes a file as another editor would, so the next save conflicts. */
  touch(id: string): string;
  /** Pretends no agent CLI is installed when given "". */
  setAnalysis(cli: string): void;
}

declare global {
  interface Window {
    agentmdMock?: MockControls;
  }
}

function headerValue(headers: HeadersInit | undefined, name: string): string | null {
  return new Headers(headers).get(name);
}

function latency(method: string, path: string): number {
  if (path === '/scan') return 450;
  if (path === '/probe') return 700;
  return method === 'GET' ? 90 : 220;
}

export function installMock(): MockServer {
  const server = new MockServer();
  const realFetch = globalThis.fetch.bind(globalThis);

  globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const raw = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
    const url = new URL(raw, document.baseURI);
    const at = url.pathname.indexOf('/api/');
    if (at === -1) return realFetch(input, init);
    const path = url.pathname.slice(at + 4);
    const method = (init?.method ?? 'GET').toUpperCase();
    const json = (status: number, body: unknown) =>
      new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

    if (method !== 'GET' && headerValue(init?.headers, 'X-Agentmd-Request') !== '1') {
      return json(403, { error: 'Every change needs the X-Agentmd-Request: 1 header.' });
    }
    let body: unknown = undefined;
    if (typeof init?.body === 'string' && init.body !== '') {
      try {
        body = JSON.parse(init.body);
      } catch {
        return json(400, { error: 'The request body is not JSON.' });
      }
    }
    await new Promise((resolve) => setTimeout(resolve, latency(method, path)));
    const res = server.handle(method, path, url.searchParams, body);
    return json(res.status, res.body);
  };

  window.agentmdMock = {
    touch: (id) => server.touch(id),
    setAnalysis: (cli) => server.setAnalysis(cli),
  };
  return server;
}
