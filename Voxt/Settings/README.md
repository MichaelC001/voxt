# Settings

Settings presentation layer for app configuration, feature tuning, permissions, models, and provider setup.

## Responsibilities

- Hosts the settings shell, general panes, dialogs, sheets, and shared settings controls.
- Separates large settings areas such as models, features, dictionary, history, onboarding, and enhancement.
- Keeps settings UI state and validation close to the screens that own it.

`PermissionsSettingsView` retains permission state, cancellable refresh/request/test tasks, and persistence. `BrowserAutomationPermissionProbes` separates their inputs/results and existing nonisolated native checks; extraction does not change the prompting or authorization policy.
