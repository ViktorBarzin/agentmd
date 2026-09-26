# HTTP API

The UI and any other client talk to `agentmd serve` over JSON. Types are in
[`internal/model/model.go`](../internal/model/model.go) and mirrored in
[`web/src/lib/types.ts`](../web/src/lib/types.ts).

Every `/api` request must send `X-Agentmd-Request: 1`, and every request with a
body sends JSON. The server never sends CORS headers. Errors answer with a
non-2xx status and `{"error": "..."}`.

| method and path | body | answer |
|---|---|---|
| `GET /api/state` | | `State`: files, references, contexts, findings |
| `POST /api/scan` | `{}` | `State` after a fresh scan |
| `GET /api/file?id=<id>` | | `{file, content}` |
| `PUT /api/file` | `{id, content, baseHash}` | `{file, content, repo, diff}`, or 409 `{error, current, hash}` when the file changed on disk since `baseHash` |
| `GET /api/git?id=<id>` | | `{repo, diff, dirty, branch, upstream, ahead, behind}` |
| `POST /api/commit` | `{ids, message}` | `{repo, commit, output}` |
| `POST /api/push` | `{id}` (any file in the repository) | `{repo, branch, upstream, output}`, or 409 with git's refusal |
| `POST /api/probe` | `{contexts: [ids]}`, empty for every unprobed or stale context | `Job` (kind `probe`, with `done` and `total`) |
| `POST /api/analyse` | `{context}` | `Job` |
| `POST /api/fix` | `{findingId}` | `Job` |
| `GET /api/jobs/<id>` | | `Job` |
| `POST /api/apply` | `{edits: [{id, content, baseHash}]}` | `{results: [{file, content, repo, diff}]}` |

- A context id is `<harness>:<dir>`, for example `claude:/home/alex/code/app`.
- A file id is the path it was found at, or `<path>#<field>` for an embedded
  file such as `/etc/claude-code/managed-settings.json#claudeMd`.
- Jobs run in the background, because probing everything takes minutes and a
  proxy may not hold a request open that long. Poll `GET /api/jobs/<id>` until
  `status` is `done` or `error`, then read `GET /api/state`. Analysis findings
  also appear in the next `State`.
- `POST /api/apply` saves several files for a fix proposal. It checks every
  `baseHash` before writing any file.
- `State.unprobed` lists the Claude Code and Codex contexts not probed yet;
  there is no static stand-in for them.
- `State.refs` holds symlink, build and mention references. Load-order
  references follow from each context's `entries`, in order.
- `POST /api/push` only fast-forwards the current branch to its upstream. It
  never rebases, merges or forces.
