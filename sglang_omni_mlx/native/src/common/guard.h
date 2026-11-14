// SPDX-License-Identifier: Apache-2.0
// Who may talk to a server: the process that started it (a per-launch
// token), never a web page (no Origin header), with bounded bodies and
// WebSocket messages. Depends on civetweb alone, so its test builds without
// MLX (ci/test_guard.sh).
#pragma once

#include <cstddef>
#include <string>

struct mg_connection;

namespace omni_server {

// The largest request body: a 64 MiB WAV is 35 minutes of 16 kHz PCM16,
// beyond a client's longest request (VoxtMini's: 60 s chunks of a
// recording, and Silero's timestamps of a whole recording of at most
// 20 minutes).
constexpr long long kMaxBodyBytes = 64LL << 20;
// The largest WebSocket message: realtime appends and VAD chunks are a few
// kilobytes.
constexpr size_t kMaxSocketMessageBytes = 1 << 20;

// Reads the token from descriptor fd: one line of 32 to 512 characters of
// RFC 6750's token68 alphabet, ending at a newline or the end of input. It
// reads byte by byte, so a later reader of the same descriptor (stdin's
// control lines) starts right after it. Throws std::invalid_argument, whose
// message starts with "--auth-token-fd must" (VoxtMini's probe).
std::string ReadToken(int fd);

// Sets the token every request must carry as "Authorization: Bearer
// <token>"; empty (the default) asks for none. Call before the server
// starts: requests read it without a lock.
void SetToken(const std::string &token);

// civetweb's begin_request callback: refuses a request with an Origin
// header (403), without the token (401), with a chunked body (411) or a
// body over kMaxBodyBytes (413), and closes the connection; 0 lets the
// request through. It runs before handlers and WebSocket upgrades.
int BeginRequest(mg_connection *connection);

// Whether a WebSocket message stays within kMaxSocketMessageBytes once a
// frame of length bytes joins the buffered ones; a handler closes the
// socket (returns 0) when it does not.
bool FitsSocketMessage(size_t buffered, size_t length);

// Constant-time equality.
bool SameSecret(const std::string &a, const std::string &b);

} // namespace omni_server
