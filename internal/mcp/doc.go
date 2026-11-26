// Package mcp serves VoxtMini's local Model Context Protocol (MCP) server
// over a Unix domain socket (run/mcp.sock, 0600 in a 0700 directory).
//
// Connections arrive from VoxtMini's stdio bridge (voxtmini-ctl mcp) and
// are identified through kernel peer credentials (LOCAL_PEERPID on macOS,
// SO_PEERCRED on Linux) rather than client-supplied strings. Every tool
// call passes through dynamic policy checks, per-agent rate limiting, a
// user-confirmation gate, and a content-free audit log.
package mcp
