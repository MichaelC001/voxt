#!/usr/bin/env python3
import json
from pathlib import Path
import tempfile
import unittest

import audit_model_stack as audit


class ModelStackAuditTests(unittest.TestCase):
    def test_current_source_has_no_retired_runtime(self):
        self.assertEqual(audit.source_errors(audit.ROOT), [])

    def test_dependency_name_detection_does_not_confuse_llama_with_onnx(self):
        for name in ["FluidAudio", "NemoTextProcessing", "sherpa-onnx", "onnxruntime"]:
            self.assertIsNotNone(audit.FORBIDDEN.search(name))
        for name in ["llama.swift", "MLXAudioVAD", "OmniVAD"]:
            self.assertIsNone(audit.FORBIDDEN.search(name))

    def test_resolved_graph_requires_expected_mlx_and_rejects_retired_runtime(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "Package.resolved"
            path.write_text(json.dumps({"pins": [
                {"identity": "fluidaudio", "location": "https://example.invalid/repo", "state": {"version": "0.15.6"}}
            ]}))
            errors = audit.resolved_errors(path)
            self.assertTrue(any("Retired resolved dependency" in error for error in errors))
            self.assertTrue(any("mlx-swift version" in error for error in errors))

    def test_missing_required_pins_are_not_a_pass(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "Package.resolved"
            path.write_text('{"pins": []}')
            self.assertTrue(audit.resolved_errors(path))


if __name__ == "__main__":
    unittest.main()
