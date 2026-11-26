-- Content-free MCP audit log: records metadata of tool calls and refused
-- connections, never user text, audio, prompts, or API keys.
CREATE TABLE mcp_calls (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    at_ms       INTEGER NOT NULL,
    client_key  TEXT    NOT NULL,
    client_name TEXT    NOT NULL,
    tool        TEXT    NOT NULL,
    scope       TEXT    NOT NULL,
    item_count  INTEGER NOT NULL DEFAULT 0,
    result      TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_mcp_calls_at ON mcp_calls (at_ms DESC, id DESC);
CREATE INDEX idx_mcp_calls_client ON mcp_calls (client_key, at_ms DESC);
