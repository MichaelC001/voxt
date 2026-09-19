"""Exercise regression command selection with a fake xcodebuild, not Swift tests."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "tools/run_local_regression_matrix.sh"


class LocalRegressionMatrixTests(unittest.TestCase):
    def run_matrix(self, group, fail_selector=""):
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            log = temp / "commands.jsonl"
            executable = temp / "xcodebuild"
            executable.write_text(
                "#!/usr/bin/env python3\n"
                "import json, os, sys\n"
                "with open(os.environ['VOXT_COMMAND_LOG'], 'a') as log:\n"
                "    log.write(json.dumps(sys.argv[1:]) + '\\n')\n"
                "sys.exit(1 if os.environ.get('VOXT_FAIL_SELECTOR') in sys.argv[1:] else 0)\n"
            )
            executable.chmod(0o755)
            environment = {
                **os.environ,
                "PATH": str(temp) + os.pathsep + os.environ.get("PATH", ""),
                "CI": "false",
                "GITHUB_ACTIONS": "false",
                "VOXT_RUN_MODEL_TESTS": "0",
                "VOXT_SPM_CACHE_PATH": str(temp / "cache"),
                "VOXT_SPM_CLONE_PATH": str(temp / "packages"),
                "VOXT_COMMAND_LOG": str(log),
                "VOXT_FAIL_SELECTOR": fail_selector,
            }
            result = subprocess.run(
                ["bash", str(SCRIPT), group], cwd=temp, env=environment,
                text=True, capture_output=True, check=False,
            )
            commands = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, commands

    def test_static_selectors_reference_existing_suites(self):
        names = set()
        for path in (ROOT / "VoxtTests").rglob("*.swift"):
            names.update(re.findall(r"^(?:final )?class (\w+)", path.read_text(), re.M))
        selectors = re.findall(r"-only-testing:VoxtTests/(\w+)", SCRIPT.read_text())
        self.assertTrue(selectors)
        self.assertEqual(set(selectors) - names, set())

    def test_refactor_selects_split_suites_with_portable_unsigned_strict_commands(self):
        result, commands = self.run_matrix("refactor")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(commands), 2)
        selectors = {arg for command in commands for arg in command if arg.startswith("-only-testing:")}
        for family in ("RemoteLLMRuntimeClient", "RemoteModelConfiguration", "HotkeyManager", "MLXModelManager", "MeetingDetailViewModel"):
            for path in (ROOT / "VoxtTests").glob(f"{family}*Tests.swift"):
                self.assertIn(f"-only-testing:VoxtTests/{path.stem}", selectors)
        for suite in ("DoubaoPacketCodecTests", "RemoteASRResponseStateTests", "RemoteASRCompletionTests", "MeetingRemoteSessionLifecycleTests"):
            self.assertIn(f"-only-testing:VoxtTests/{suite}", selectors)
        for suite in ("TrackedTaskStoreTests", "LLMRequestLifecycleTests", "MeetingLiveSessionRegistryTests", "MLXCorrectionPassCoordinatorTests", "MeetingCaptureTimelineTests", "MLXNativeLiveRuntimeTests", "HotkeyEventTapRunLoopTests", "HotkeyManagerLifetimeTests"):
            self.assertIn(f"-only-testing:VoxtTests/{suite}", selectors)
        for command in commands:
            self.assertEqual(command[0], "test")
            self.assertIn(str(ROOT / "Voxt.xcodeproj"), command)
            self.assertIn("CODE_SIGNING_ALLOWED=NO", command)
            self.assertIn("-onlyUsePackageVersionsFromResolvedFile", command)

    def test_all_does_not_run_core_vad_suites_twice(self):
        result, commands = self.run_matrix("all")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(commands), 3)
        selectors = [arg for command in commands for arg in command if arg.startswith("-only-testing:")]
        self.assertEqual(len(selectors), len(set(selectors)))

    def test_removed_groups_fail_instead_of_selecting_missing_tests(self):
        for group in ("whisper", "diagnostic", "unknown"):
            with self.subTest(group=group):
                result, commands = self.run_matrix(group)
                self.assertEqual(result.returncode, 2)
                self.assertEqual(commands, [])

    def test_later_success_does_not_hide_core_failure(self):
        result, commands = self.run_matrix(
            "refactor", "-only-testing:VoxtTests/TranscriptionCapturePipelineTests"
        )
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(commands), 2)
        self.assertIn("Group failed: run_core", result.stdout)


if __name__ == "__main__":
    unittest.main()
