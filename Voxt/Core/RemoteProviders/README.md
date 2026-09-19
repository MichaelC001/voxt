# Remote Providers

Configuration and connectivity support for remote ASR, LLM, and provider-specific dictionary services.

## Responsibilities

- Defines provider configuration models, validation policy, and secure settings handling.
- Builds provider-specific payloads and endpoints for connectivity checks and dictionary requests.
- Centralizes remote provider diagnostics, request logging, and availability testing.

Model-facing option values live in `RemoteProviderModelOptions`; model/default/endpoint normalization lives in `RemoteProviderConfigurationResolution`. Credential presence, migration/storage, the Codable configuration and runtime-wrapper construction remain together in `RemoteProviderConfiguration`.

The credential-presence type/field and runtime-wrapper initializer intentionally remain `fileprivate`. Do not widen that boundary just to split a file; the resolution extension must not access Keychain or manufacture runtime credential wrappers.
