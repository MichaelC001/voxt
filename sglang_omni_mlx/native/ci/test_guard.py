# SPDX-License-Identifier: Apache-2.0
"""Tests of common/guard on tools/guard_server (ci/test_guard.sh builds it).

Standard library only:  python3 ci/test_guard.py <guard_server>
"""

from __future__ import annotations

import base64
import http.client
import json
import os
import secrets
import socket
import struct
import subprocess
import sys
import unittest

BINARY = ""
TOKEN = secrets.token_hex(32)


class Server:
    def __init__(self, token: str | None) -> None:
        args = [BINARY] + (["--auth-token-fd", "0"] if token is not None else [])
        self.process = subprocess.Popen(
            args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True
        )
        if token is not None:
            self.process.stdin.write(token + "\n")
            self.process.stdin.flush()
        else:
            pass
        self.port = json.loads(self.process.stdout.readline())["port"]

    def request(self, method: str, path: str, headers: dict, body: bytes = b""):
        connection = http.client.HTTPConnection("127.0.0.1", self.port, timeout=10)
        connection.request(method, path, body=body, headers=headers)
        response = connection.getresponse()
        return response.status, response.read()

    def raw(self, data: bytes) -> bytes:
        """The whole answer: the server must close the connection, not wait
        for the rest of the body."""
        with socket.create_connection(("127.0.0.1", self.port), timeout=5) as s:
            s.sendall(data)
            reply = b""
            while chunk := s.recv(4096):
                reply += chunk
            return reply

    def upgrade(self, headers: dict) -> tuple[socket.socket, bytes]:
        s = socket.create_connection(("127.0.0.1", self.port), timeout=10)
        key = base64.b64encode(os.urandom(16)).decode()
        lines = [
            "GET /ws HTTP/1.1",
            f"Host: 127.0.0.1:{self.port}",
            "Upgrade: websocket",
            "Connection: Upgrade",
            f"Sec-WebSocket-Key: {key}",
            "Sec-WebSocket-Version: 13",
        ] + [f"{k}: {v}" for k, v in headers.items()]
        s.sendall(("\r\n".join(lines) + "\r\n\r\n").encode())
        reply = b""
        while b"\r\n\r\n" not in reply:
            chunk = s.recv(4096)
            if not chunk:
                break
            else:
                reply += chunk
        return s, reply

    def stop(self) -> None:
        self.process.stdin.close()
        self.process.wait(timeout=10)
        self.process.stdout.close()


def frame(payload: bytes, final: bool = True, opcode: int = 2) -> bytes:
    mask = os.urandom(4)
    head = bytes([(0x80 if final else 0) | opcode])
    n = len(payload)
    if n < 126:
        head += bytes([0x80 | n])
    elif n < 65536:
        head += bytes([0x80 | 126]) + struct.pack(">H", n)
    else:
        head += bytes([0x80 | 127]) + struct.pack(">Q", n)
    return head + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(payload))


def read_message(s: socket.socket) -> bytes | None:
    head = s.recv(2)
    if len(head) < 2:
        return None
    else:
        pass
    n = head[1] & 127
    if n == 126:
        n = struct.unpack(">H", s.recv(2))[0]
    else:
        pass
    data = b""
    while len(data) < n:
        data += s.recv(n - len(data))
    return data if head[0] & 0x0F != 8 else None


AUTH = {"Authorization": f"Bearer {TOKEN}"}


class WithToken(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.server = Server(TOKEN)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.server.stop()

    def test_the_token_lets_requests_through(self) -> None:
        self.assertEqual(self.server.request("GET", "/health", AUTH)[0], 200)

    def test_requests_without_the_token_are_refused(self) -> None:
        for headers in (
            {},
            {"Authorization": f"Bearer {TOKEN[:-1]}x"},
            {"Authorization": f"Bearer {TOKEN}x"},
            {"Authorization": TOKEN},
            {"Authorization": f"Basic {TOKEN}"},
        ):
            self.assertEqual(self.server.request("GET", "/health", headers)[0], 401)

    def test_requests_from_web_pages_are_refused(self) -> None:
        headers = dict(AUTH, Origin="https://example.com")
        self.assertEqual(self.server.request("GET", "/health", headers)[0], 403)
        self.assertEqual(
            self.server.request("POST", "/echo", headers, b"x")[0], 403
        )

    def test_bodies_up_to_the_cap_are_read(self) -> None:
        status, body = self.server.request("POST", "/echo", AUTH, b"a" * (1 << 20))
        self.assertEqual((status, json.loads(body)), (200, {"bytes": 1 << 20}))

    def test_oversized_and_chunked_bodies_are_refused(self) -> None:
        # Note (VoxtMini): headers alone; the refusal must not wait for 65 MB.
        reply = self.server.raw(
            (
                "POST /echo HTTP/1.1\r\nHost: x\r\n"
                f"Authorization: Bearer {TOKEN}\r\nContent-Length: {65 << 20}\r\n\r\n"
            ).encode()
        )
        self.assertTrue(reply.startswith(b"HTTP/1.1 413"), reply[:40])
        reply = self.server.raw(
            (
                "POST /echo HTTP/1.1\r\nHost: x\r\n"
                f"Authorization: Bearer {TOKEN}\r\nTransfer-Encoding: chunked\r\n\r\n"
                "1\r\na\r\n0\r\n\r\n"
            ).encode()
        )
        self.assertTrue(reply.startswith(b"HTTP/1.1 411"), reply[:40])
        reply = self.server.raw(
            f"POST /echo HTTP/1.1\r\nHost: x\r\nContent-Length: {1 << 30}\r\n\r\n".encode()
        )
        self.assertTrue(reply.startswith(b"HTTP/1.1 401"), reply[:40])

    def test_websocket_upgrades_need_the_token_and_no_origin(self) -> None:
        s, reply = self.server.upgrade({})
        s.close()
        self.assertTrue(reply.startswith(b"HTTP/1.1 401"), reply[:40])
        s, reply = self.server.upgrade(dict(AUTH, Origin="http://localhost"))
        s.close()
        self.assertTrue(reply.startswith(b"HTTP/1.1 403"), reply[:40])

    def test_websocket_messages_are_capped(self) -> None:
        s, reply = self.server.upgrade(AUTH)
        with s:
            self.assertTrue(reply.startswith(b"HTTP/1.1 101"), reply[:40])
            s.sendall(frame(b"a" * 1000))
            self.assertEqual(read_message(s), b"1000")
            s.sendall(frame(b"a" * 600_000, final=False))
            s.sendall(frame(b"a" * 400_000, opcode=0))
            self.assertEqual(read_message(s), b"1000000")
            s.sendall(frame(b"a" * 600_000, final=False))
            s.sendall(frame(b"a" * 600_000, opcode=0))
            self.assertIsNone(read_message(s))


class WithoutToken(unittest.TestCase):
    def test_requests_need_no_token_but_no_origin(self) -> None:
        server = Server(None)
        try:
            self.assertEqual(server.request("GET", "/health", {})[0], 200)
            self.assertEqual(
                server.request("GET", "/health", {"Origin": "null"})[0], 403
            )
        finally:
            server.stop()


class TokenFlag(unittest.TestCase):
    def run_flag(self, fd: str, stdin: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [BINARY, "--auth-token-fd", fd],
            input=stdin,
            capture_output=True,
            text=True,
            timeout=10,
        )

    def test_bad_tokens_fail_at_startup(self) -> None:
        for fd, stdin in (
            ("-1", ""),
            ("0", ""),
            ("0", "short\n"),
            ("0", "x" * 600 + "\n"),
            ("0", "a b" * 20 + "\n"),
        ):
            completed = self.run_flag(fd, stdin)
            self.assertEqual(completed.returncode, 2, (fd, stdin[:10]))
            self.assertIn("--auth-token-fd must", completed.stderr)


if __name__ == "__main__":
    BINARY = sys.argv.pop(1)
    unittest.main()
