# MLX Audio Dependency Policy

Voxt uses `mlx-audio-swift` through the mirror fork at `https://github.com/hehehai/mlx-audio-swift`.

The current Xcode package reference is:

- URL: `https://github.com/hehehai/mlx-audio-swift.git`
- Requirement: `exactVersion`
- Version: `0.1.3-voxt.12`

## Version rules

- Prefer upstream release tags when they already contain the STT features or fixes Voxt needs.
- When upstream `main` contains required changes that are not released yet, sync the fork's `main` to upstream and create a Voxt tag on the selected commit.
- Switch Voxt back to upstream release tags once an official release covers the same changes.

## Tag rules

- Keep the fork as a mirror plus tags only. Do not land Voxt-specific source patches there unless absolutely required.
- Use tags in the form `v<upstream-version>-voxt.<n>`.
- Do not reuse upstream tag names for different commits.

## Update workflow

1. Sync `hehehai/mlx-audio-swift` `main` from `Blaizzy/mlx-audio-swift`.
2. Pick the target commit from fork `main`.
3. Create a new annotated Voxt tag on that commit, for example `v0.1.2-voxt.2`.
4. Point `Voxt.xcodeproj` at the fork URL and `exactVersion`.
5. Build Voxt and verify STT model loading, legacy repo migration, and downloaded-model detection before shipping.

If Voxt needs to consume a synced fork commit before a new tag exists, pin the project to that exact `revision` temporarily, then switch back to a release tag once the fork tag is cut.

## Practical rules for Voxt maintainers

- Use upstream releases directly when they already include the models or fixes Voxt needs.
- Use the fork only when Voxt must consume unreleased upstream commits.
- Keep the fork as a mirror plus tags. Do not put long-lived Voxt-only API changes into the fork.
- Once upstream publishes an official release that covers the same changes, switch Voxt back to the upstream release tag instead of staying on a fork tag forever.
- If a new MLX Audio update renames model repos, add canonical mapping in `MLXModelManager` so existing user settings and downloaded caches continue to work.

## Current pin

- Fork: `hehehai/mlx-audio-swift`
- Requirement: `exactVersion`
- Version: `0.1.3-voxt.12`
- Commit: `95b4587`
- Notes: `TranscriptionEvent.ended` carries full `STTOutput` (text / segments / language provenance) for Qwen, MOSS, Cohere, and Nemotron live stop, aligned with batch structured-output semantics; includes upstream through `c5d4054` and Voxt Qwen KV / language-parameter work

## Dependency verification on the modernization branch

FluidAudio has been removed from the app's package graph and speaker-analysis implementation. Sortformer remains in `MLXAudioVAD`; this does not imply that FluidAudio was an ONNX dependency.

Run `bash tools/resolve_dependencies.sh` on macOS to generate or verify the workspace `Package.resolved`, then review and commit that file. It is no longer ignored at the app workspace path. Tests preserve the generated graph as an artifact; releases require the committed graph and audit both dynamic links and static link maps. Source-only checks: `python3 tools/audit_model_stack.py`.

The audio fork at `v0.1.3-voxt.12` explicitly pins `mlx-swift` to **exact 0.31.4**. A new LM revision requiring 0.31.6 cannot be upgraded independently. Also preserve the fork's structured `.ended(STTOutput)`, Qwen language/KV, and Nemotron streaming changes before switching to upstream; syncing upstream alone is not a verified replacement.

The modernization work was performed on Linux without Xcode. No macOS build, model benchmark or newly resolved lockfile has been verified there. The MLX compatibility set below is deliberately unchanged pending a compatible audio fork and Swift 6.3 validation. See [implementation status](ModelStackModernizationImplementation.zh-CN.md).

## Local upgrade candidate (not the release pin)

The sibling `../mlx-audio-swift` checkout now has a local branch `chore/voxt-model-stack-modernization`, based on `v0.1.3-voxt.12` with upstream content through `3e978558404df4ad1bbb0a5634a03df2b0f9dfa5`. It preserves Voxt's structured ended output and streaming patches. The candidate is recorded in local commit `0bfe9f31e6473b461f3bdf2c3382f2478792976e`; no release tag has been created and nothing has been pushed.

Its manifest requires Swift 6.3 and pins:

- `mlx-swift` exact `0.31.6`
- `mlx-swift-lm` revision `c6446cf7bfb7cea76408013b614d4b2c530eaa03`
- `swift-transformers` exact `1.3.4`
- `swift-huggingface` exact `0.10.2`

The old audio lockfile was removed pending real Xcode resolution. See the fork's `VOXT_UPGRADE.md` for API changes and regression gates. Do not replace the app's release reference with an unpublished tag or a personal absolute path.

### Build an isolated Voxt candidate

On the Mac with both repos checked out, create a new directory outside the source repos:

```bash
python3 tools/prepare_mlx_upgrade.py \
  --audio ../mlx-audio-swift --output /tmp/voxt-mlx-upgrade
xcodebuild -resolvePackageDependencies \
  -project /tmp/voxt-mlx-upgrade/Voxt.xcodeproj -scheme Voxt
xcodebuild test -project /tmp/voxt-mlx-upgrade/Voxt.xcodeproj -scheme Voxt \
  -destination 'platform=macOS' -onlyUsePackageVersionsFromResolvedFile \
  CODE_SIGNING_ALLOWED=NO
```

This copies app/test sources, excludes personal signing config and stale lockfiles, substitutes a local Audio reference and the matching LM revision, and applies `tools/mlx-next.patch` in the copy only. The patch retains prequantized loading, calls the new model preparation lifecycle, uses model-declared chat conventions/effective EOS, and replaces deprecated prefill access with the typed API (legacy chunk boundaries retained for comparison).

The generator is tested on Linux; successful snapshot generation is **not** a successful Swift build. Validate Swift/Xcode, model quality, reasoning/EOS behavior and peak memory on Apple Silicon. Only then publish the fork candidate, update both production package pins, apply the adapter to production sources, regenerate the lockfile and update `tools/audit_model_stack.py`. The candidate snapshot must not be released directly.

## Current MLX compatibility set

The project / audio fork require:

- `mlx-swift-lm` at revision `d2424294a6c3bbd0de37a0761d80efc05e6813dd` (direct Voxt pin)
- `mlx-swift` `0.31.4` (transitive via the audio/lm graph)

Toolchain ceiling documented for the earlier Swift 6.2 / Xcode 26.3 baseline (CI now requests Xcode 26.5; verify its actual Swift version):

- `mlx-swift` 0.31.5+ requires `swift-tools-version: 6.3`, so SPM keeps `0.31.4` even though `0.31.6` exists.
- `mlx-swift-lm` commits after `d242429` need APIs from `mlx-swift` ≥ 0.31.5 (`greatestFiniteMagnitudeArray`, later `maskFill` / TurboQuant). Keep `d242429` until the app can move to a Swift 6.3 toolchain, then update the audio fork's exact dependency and bump `mlx-swift` to ≥ 0.31.6 with a tested, immutable `mlx-swift-lm` revision together.
