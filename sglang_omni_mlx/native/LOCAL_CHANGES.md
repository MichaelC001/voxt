# VoxtMini's changes to the pinned runtime

This tree is SGLang-Omni's native MLX runtime at the commit in
[`UPSTREAM_PIN`](UPSTREAM_PIN), with the changes below. Keep this list
current when you change the tree; re-apply them when you move the pin.

## The servers' guard (2026-11-14)

The servers listen on a loopback port, which every process on the Mac and
every web page in a browser can reach. `src/common/guard.{h,cpp}`, installed
as civetweb's `begin_request` callback in `asr_service::Serve` (so in every
server binary), checks each request and WebSocket handshake before a handler
sees it:

- `--auth-token-fd FD`: the server reads a token from descriptor `FD` at
  startup (VoxtMini: `0`, the first line of stdin, before the supervisor's
  control lines; never an argument, which `ps` shows to every user, nor an
  environment variable). Every request must then carry
  `Authorization: Bearer <token>` (constant-time compare), else 401. Without
  the flag no token is required (the CI scripts, by hand). A bad descriptor or
  token fails at startup with `--auth-token-fd must …`, before loading the
  model: VoxtMini probes a binary with `--auth-token-fd -1` and runs an older
  one without a token.
- A request with an `Origin` header gets 403, token or not: browsers send one
  with every WebSocket handshake and POST, the owner never does.
- A chunked body gets 411, a `Content-Length` over 64 MiB 413 (`kMaxBodyBytes`;
  `ReadBody` also stops there); refusals close the connection without reading
  the body.
- A realtime WebSocket message over 1 MiB closes the socket
  (`kMaxSocketMessageBytes`; the VAD and diarization streams have their own,
  smaller caps).

`ci/test_guard.sh` builds the guard with the pinned civetweb and a small
server (`tools/guard_server.cpp`) with the system's compilers, no MLX, and
runs `ci/test_guard.py`: it runs on Linux too. `ci/test_server_api.py` tests
the real server.

## civetweb past v1.16

`CMakeLists.txt` pins civetweb to master `d110e87` (v1.16 + 270 commits;
civetweb has made no release since v1.16, 2023). It contains the fix of
CVE-2025-55763 (a heap overflow in the directory URI redirect of releases
1.14 to 1.16: PR #1347, merged as `c584455`) and of the WebSocket payload's
allocation (`d110e87`). Not the next commit, `588860e` (it does
not compile). The guard refuses chunked bodies itself, which that commit's
chunked-and-length check was about.

## Earlier

- `--cache-limit-mb` (`qwen3_asr_server`): MLX's buffer cache limit within a
  request (VoxtMini's docs/behavior.md 3.5a).
