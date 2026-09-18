import tempfile
from pathlib import Path
import shutil
import unittest

import prepare_mlx_upgrade as upgrade


class LocalMLXUpgradeTests(unittest.TestCase):
    def test_project_override_is_local_and_pins_lm_without_touching_release_reference(self):
        original = (upgrade.ROOT / "Voxt.xcodeproj/project.pbxproj").read_text()
        candidate = upgrade.candidate_project(original, Path('/tmp/audio checkout'))
        self.assertIn('relativePath = "/tmp/audio checkout";', candidate)
        self.assertIn('isa = XCLocalSwiftPackageReference;', candidate)
        self.assertIn(upgrade.LM_REVISION, candidate)
        self.assertNotIn('repositoryURL = "https://github.com/hehehai/mlx-audio-swift.git";', candidate)
        self.assertEqual((upgrade.ROOT / "Voxt.xcodeproj/project.pbxproj").read_text(), original)

    def test_snapshot_applies_new_api_adapter_without_editing_original_files(self):
        with tempfile.TemporaryDirectory() as directory:
            parent = Path(directory)
            source, audio, output = parent / "source", parent / "audio", parent / "candidate"
            for name in ["Voxt", "VoxtTests", "Voxt.xcodeproj", "Config", "tools"]:
                (source / name).mkdir(parents=True)
            audio.mkdir()
            (audio / "Package.swift").write_text(
                f'// swift-tools-version:6.3\nexact: "0.31.6"\nrevision: "{upgrade.LM_REVISION}"'
            )
            files = [
                "Voxt.xcodeproj/project.pbxproj", "tools/mlx-next.patch",
                "Voxt/Core/Models/MemoryEfficientModelContainerLoader.swift",
                "Voxt/Core/Models/CustomLLMModelManager.swift",
            ]
            for name in files:
                destination = source / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(upgrade.ROOT / name, destination)
            (source / "Config/Signing.local.xcconfig").write_text("personal signing")
            project = upgrade.prepare(output, audio, root=source)
            self.assertTrue(project.is_dir())
            self.assertFalse((output / "Config/Signing.local.xcconfig").exists())
            loader = (output / files[2]).read_text()
            self.assertNotIn("ToolCallFormat.infer(", loader)
            self.assertIn("try model.prepare()", loader)
            self.assertIn("PrequantizedModelLoading.replaceLayers", loader)
            for name in files:
                self.assertEqual((source / name).read_bytes(), (upgrade.ROOT / name).read_bytes())
            with self.assertRaises(ValueError):
                upgrade.prepare(output, audio, root=source)

    def test_rejects_old_audio_checkout(self):
        with tempfile.TemporaryDirectory() as directory:
            audio = Path(directory) / "audio"
            audio.mkdir()
            (audio / "Package.swift").write_text('// swift-tools-version:6.2')
            output = Path(directory) / "candidate"
            with self.assertRaises(ValueError):
                upgrade.prepare(output, audio)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
