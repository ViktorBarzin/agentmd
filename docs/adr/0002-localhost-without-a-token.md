# Localhost without a token, a shared secret behind a proxy

`agentmd serve` listens on loopback and asks for no token, because it is meant to
be run by one person on their own machine with one command. It still defends
itself against the ways a loopback server gets reached by someone else. It
accepts only a loopback `Host` header, so DNS rebinding fails. It requires a
custom request header on every change, so another website cannot post to it. On
Linux it looks up the connecting socket's owner in `/proc/net/tcp`, and refuses
connections from other OS users on a shared machine.

Listening on any other address requires a proxy secret. In that mode every
request must carry the secret in `X-Agentmd-Proxy-Secret` and name an allowed
identity in the proxy's user header. The process always acts as the OS user who
started it. This is how a homelab puts agentmd behind an auth proxy such as
Authentik, and the proxy-specific configuration lives with that deployment, not
in this repository.

## Considered options

- **A random token in the URL** (the Jupyter model). It protects against other
  local users on every platform. It was turned down because it makes the tool
  harder to run and bookmark, and the peer-owner check covers shared Linux
  machines.
- **A Unix socket.** Browsers cannot connect to one without a helper.
