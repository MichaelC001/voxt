# Documentation

VoxtMini's documents, by reader. Some are in Chinese (中文), as marked; where a design document and the code disagree, the code and [AGENTS.md](../AGENTS.md) win.

## For users

- [how-it-works.md](how-it-works.md): features in detail, the models, requirements, downloads, privacy, Voxt's history (en + 中文).
- [CHANGELOG.md](../CHANGELOG.md): what changed in each version (en + 中文).

## For contributors

- [CONTRIBUTING.md](../CONTRIBUTING.md): how to send a change (en + 中文).
- [AGENTS.md](../AGENTS.md): the rules for every change (strings, threading, protocols, file sizes, licences) and all commands.
- [development.md](development.md): building from source, signing during development, the tests.
- [architecture.md](architecture.md): packages, processes, threading, data on disk, testing.
- [behavior.md](behavior.md): what VoxtMini does differently from Voxt, and why (中文).
- [helper-protocol.md](helper-protocol.md): the protocol between VoxtMini and its Swift helper.
- [llm-protocol.md](llm-protocol.md): the protocol between VoxtMini and `voxtmini-llm`, the language models' process.
- [llm-models.md](llm-models.md): the language models, their generation settings and measurements.
- [eval.md](eval.md): the models' quality on an eval set, and the decisions it led to.
- [manual-test.md](manual-test.md): the checks by hand on a Mac, and the automated Mac tests (中文).
- [release.md](release.md): building, signing, notarising and publishing a release; updates.

## Design notes

- [plan.md](plan.md): what VoxtMini is, its runtimes and milestones.
- [models-page.md](models-page.md): the Models page, local and remote models in one place.
- [remote-models.md](remote-models.md): remote speech and language services, chosen per feature.
- [apple-models.md](apple-models.md): Apple Speech and Apple Intelligence as local models.

## History

- [history/README.md](history/README.md): documents kept for the record; newer decisions override them.
- [history/voxt-migration.md](history/voxt-migration.md): the decision to rebuild Voxt as VoxtMini (2026-10-18).
- [history/voxt-gap-analysis.md](history/voxt-gap-analysis.md): what Voxt had that VoxtMini lacked (2026-11-11, 中文).

## Audits

- [audit/package-split-dead-code.md](audit/package-split-dead-code.md): package split, dead code, big files, test speed (2026-11-14).
- [audit/performance.md](audit/performance.md): CPU, memory, disk, energy, network and start-up (2026-11-14, 中文).
