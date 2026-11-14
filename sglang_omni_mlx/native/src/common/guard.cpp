// SPDX-License-Identifier: Apache-2.0
#include "common/guard.h"

#include <unistd.h>

#include <cerrno>
#include <cstring>
#include <stdexcept>

#include "civetweb.h"

namespace omni_server {

namespace {

std::string token;

constexpr size_t kMinToken = 32;
constexpr size_t kMaxToken = 512;

bool TokenCharacter(char c) {
  return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
         (c >= '0' && c <= '9') || std::strchr("-._~+/=", c) != nullptr;
}

int Refuse(mg_connection *connection, int status, const char *detail) {
  // Note (VoxtMini): mg_send_http_error closes the connection, so an unread
  // body is never drained, but it sends no status line on a WebSocket
  // handshake (a GET without a body), which gets its answer written here.
  if (mg_get_header(connection, "Upgrade") == nullptr) {
    mg_send_http_error(connection, status, "%s", detail);
  } else {
    mg_printf(connection,
              "HTTP/1.1 %d %s\r\nContent-Type: text/plain\r\n"
              "Content-Length: %zu\r\nConnection: close\r\n\r\n%s",
              status, mg_get_response_code_text(connection, status),
              std::strlen(detail), detail);
  }
  return status;
}

} // namespace

std::string ReadToken(int fd) {
  const std::string flag = "--auth-token-fd";
  std::string line;
  for (;;) {
    char c = 0;
    const ssize_t n = read(fd, &c, 1);
    if (n < 0 && errno == EINTR) {
      continue;
    } else if (n < 0) {
      throw std::invalid_argument(flag +
                                  " must be an open file descriptor: " +
                                  std::strerror(errno));
    } else if (n == 0 || c == '\n') {
      break;
    } else if (line.size() > kMaxToken) {
      throw std::invalid_argument(flag + " must give at most " +
                                  std::to_string(kMaxToken) + " characters");
    } else {
      line.push_back(c);
    }
  }
  if (!line.empty() && line.back() == '\r') {
    line.pop_back();
  } else {
  }
  if (line.size() < kMinToken || line.size() > kMaxToken) {
    throw std::invalid_argument(flag + " must give a token of " +
                                std::to_string(kMinToken) + " to " +
                                std::to_string(kMaxToken) + " characters");
  } else {
  }
  for (const char c : line) {
    if (!TokenCharacter(c)) {
      throw std::invalid_argument(flag +
                                  " must give a token of letters, digits and "
                                  "-._~+/=");
    } else {
    }
  }
  return line;
}

void SetToken(const std::string &value) { token = value; }

bool SameSecret(const std::string &a, const std::string &b) {
  if (a.size() != b.size()) {
    return false;
  } else {
  }
  volatile unsigned char difference = 0;
  for (size_t i = 0; i < a.size(); ++i)
    difference = difference | static_cast<unsigned char>(a[i] ^ b[i]);
  return difference == 0;
}

int BeginRequest(mg_connection *connection) {
  // Note (VoxtMini): browsers send Origin with every WebSocket handshake and
  // every POST; the owner never does. Refusing it keeps web pages out even
  // without a token.
  if (mg_get_header(connection, "Origin") != nullptr) {
    return Refuse(connection, 403, "Requests from web pages are refused.");
  } else {
  }
  if (!token.empty()) {
    const char *authorization = mg_get_header(connection, "Authorization");
    if (authorization == nullptr ||
        !SameSecret(authorization, "Bearer " + token)) {
      return Refuse(connection, 401, "Unauthorized.");
    } else {
    }
  } else {
  }
  if (mg_get_header(connection, "Transfer-Encoding") != nullptr) {
    return Refuse(connection, 411, "A Content-Length is required.");
  } else {
  }
  if (mg_get_request_info(connection)->content_length > kMaxBodyBytes) {
    return Refuse(connection, 413, "The request body is too large.");
  } else {
  }
  return 0;
}

bool FitsSocketMessage(size_t buffered, size_t length) {
  return length <= kMaxSocketMessageBytes &&
         buffered <= kMaxSocketMessageBytes - length;
}

} // namespace omni_server
