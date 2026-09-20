# Feature Settings

Feature-level settings UI for transcription, translation, meeting, shortcuts, prompts, and model selection.

## Responsibilities

- Presents feature cards, rows, section layouts, and model selector dialogs.
- Manages prompt drafts and feature settings state used across multiple feature panes.
- Groups related feature controls without mixing them into the settings shell.

## Files page layout

`FeatureFileTaskSections.filesContent` uses a scrolling task list above a pinned upload card. The settings shell owns the outer 16-point horizontal and bottom insets; this page does not add a page-wide trailing scroll gutter or extra bottom padding. Empty-state/task cards fill the list's available width, while the upload card spans the page content width. The upload card's internal padding is unchanged.

After layout changes, verify on macOS at the minimum window size and a larger size, with both empty and populated/scrolling queues. Check equal left/right outer margins, a 16-point bottom gap, visible task actions, and that uploading/dropping files remains accessible. Also check the system's always-visible scrollbar setting. Visual validation is pending; these checks cannot run in the Linux development environment.
