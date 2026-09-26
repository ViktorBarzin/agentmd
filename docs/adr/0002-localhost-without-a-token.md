# Localhost without a token, a shared secret behind a proxy

`agentmd serve` listens on loopback and asks for no token, because it is meant to
be run by one person on their own machine with one command. It still defends
itself against the ways a loopback server gets reached by someone else:

- It accepts only a loopback `Host` header, so DNS rebinding fails.
- Every `/api` call must carry a custom request header, and the server never
  sends CORS headers, so another website can neither post to it nor start a
  probe or an analysis, which run processes and spend the owner's plan.
- On Linux it looks up the connecting socket's owner in `/proc/net/tcp`, and
  refuses connections from other OS users on a shared machine. A relay the owner
  runs themselves counts as the owner.

Listening on any other address requires a proxy secret. In that mode every
request must carry the secret in `X-Agentmd-Proxy-Secret` and name an allowed
identity in the proxy's user header. The process always acts as the OS user who
started it. `serve` also accepts a listening socket from systemd, so on a shared
machine the service manager holds the port and no other user can take it during
a restart. This is how a homelab puts agentmd behind an auth proxy such as
Authentik; the proxy-specific configuration lives with that deployment, not in
this repository.

## Considered options

- **A random token in the URL** (the Jupyter model). It protects against other
  local users on every platform. It was turned down because it makes the tool
  harder to run and bookmark, and the peer-owner check covers shared Linux
  machines.
- **A Unix socket.** Browsers cannot connect to one without a helper.
