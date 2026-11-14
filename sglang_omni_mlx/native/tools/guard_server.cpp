// SPDX-License-Identifier: Apache-2.0
// A server on common/guard alone, for ci/test_guard.py: no model, no MLX, so
// it builds on Linux too (ci/test_guard.sh).
//
//   guard_server [--auth-token-fd FD]
//
// It prints {"port": N} once it listens and stops at the end of stdin.
// GET /health answers {"status":"healthy"}; POST /echo the number of body
// bytes it read; WebSocket /ws each message's length.
#include <cstdio>
#include <cstring>
#include <iostream>
#include <stdexcept>
#include <string>

#include "civetweb.h"
#include "common/guard.h"

namespace {

int Reply(mg_connection *connection, const std::string &body) {
  mg_printf(connection,
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n"
            "Content-Length: %zu\r\nConnection: close\r\n\r\n%s",
            body.size(), body.c_str());
  return 200;
}

int Health(mg_connection *connection, void *) {
  return Reply(connection, "{\"status\":\"healthy\"}");
}

int Echo(mg_connection *connection, void *) {
  char buffer[65536];
  long long total = 0;
  int read = 0;
  while ((read = mg_read(connection, buffer, sizeof(buffer))) > 0)
    total += read;
  return Reply(connection, "{\"bytes\":" + std::to_string(total) + "}");
}

int SocketData(mg_connection *connection, int bits, char *, size_t length,
               void *) {
  auto *buffered =
      static_cast<size_t *>(mg_get_user_connection_data(connection));
  const int opcode = bits & 0x0F;
  if (opcode == MG_WEBSOCKET_OPCODE_CONNECTION_CLOSE) {
    return 0;
  } else if (opcode == MG_WEBSOCKET_OPCODE_PING ||
             opcode == MG_WEBSOCKET_OPCODE_PONG) {
    return 1;
  } else {
  }
  if (!omni_server::FitsSocketMessage(*buffered, length)) {
    return 0;
  } else {
  }
  *buffered += length;
  if ((bits & 0x80) == 0) {
    return 1;
  } else {
  }
  const std::string reply = std::to_string(*buffered);
  *buffered = 0;
  mg_websocket_write(connection, MG_WEBSOCKET_OPCODE_TEXT, reply.data(),
                     reply.size());
  return 1;
}

void SocketReady(mg_connection *connection, void *) {
  mg_set_user_connection_data(connection, new size_t(0));
}

void SocketClosed(const mg_connection *connection, void *) {
  delete static_cast<size_t *>(mg_get_user_connection_data(connection));
}

} // namespace

int main(int argc, char **argv) {
  std::string token;
  for (int i = 1; i < argc; ++i) {
    if (std::strcmp(argv[i], "--auth-token-fd") == 0 && i + 1 < argc) {
      try {
        token = omni_server::ReadToken(std::atoi(argv[++i]));
      } catch (const std::exception &error) {
        std::cerr << argv[0] << ": " << error.what() << "\n";
        return 2;
      }
    } else {
      std::cerr << argv[0] << ": unknown argument " << argv[i] << "\n";
      return 2;
    }
  }
  omni_server::SetToken(token);
  mg_init_library(0);
  const char *options[] = {"listening_ports", "127.0.0.1:0", "num_threads",
                           "8", nullptr};
  mg_callbacks callbacks{};
  callbacks.begin_request = omni_server::BeginRequest;
  mg_context *context = mg_start(&callbacks, nullptr, options);
  if (context == nullptr) {
    std::cerr << "cannot listen\n";
    return 1;
  } else {
  }
  mg_set_request_handler(context, "/health$", Health, nullptr);
  mg_set_request_handler(context, "/echo$", Echo, nullptr);
  mg_set_websocket_handler(context, "/ws", nullptr, SocketReady, SocketData,
                           SocketClosed, nullptr);
  mg_server_port port{};
  mg_get_server_ports(context, 1, &port);
  std::cout << "{\"port\": " << port.port << "}" << std::endl;
  std::string line;
  while (std::getline(std::cin, line)) {
  }
  mg_stop(context);
  return 0;
}
