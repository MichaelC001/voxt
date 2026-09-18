#!/usr/bin/env python3
"""Create an isolated Voxt project using the local MLX Audio upgrade candidate.

Does not edit the source checkout, its dependency pins, or its lockfile. Requires
an empty/nonexistent output path. Build the result with Xcode + Swift 6.3 on Mac.
"""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

ROOT = Path(__file__).resolve().parent.parent
LM_REVISION = "c6446cf7bfb7cea76408013b614d4b2c530eaa03"


def replace_once(text, pattern, replacement):
    result, count = re.subn(pattern, lambda _: replacement, text, flags=re.DOTALL)
    if count != 1:
        raise ValueError(f"Expected one project reference, found {count}: {pattern}")
    return result


def candidate_project(text, audio):
    audio_pattern = (
        r'\t\tA1B2C3D4E5F607890123456B /\* XCRemoteSwiftPackageReference "mlx-audio-swift" \*/ = \{'
        r'.*?\n\t\t\};'
    )
    text = replace_once(text, audio_pattern, "")
    local = (
        '/* Begin XCLocalSwiftPackageReference section */\n'
        '\t\tA1B2C3D4E5F607890123456B /* XCLocalSwiftPackageReference "mlx-audio-swift" */ = {\n'
        '\t\t\tisa = XCLocalSwiftPackageReference;\n'
        f'\t\t\trelativePath = {json.dumps(str(audio), ensure_ascii=False)};\n'
        '\t\t};\n'
        '/* End XCLocalSwiftPackageReference section */\n\n'
    )
    marker = '/* Begin XCRemoteSwiftPackageReference section */'
    if text.count(marker) != 1:
        raise ValueError("Missing remote package section")
    text = text.replace(marker, local + marker)
    text = text.replace('XCRemoteSwiftPackageReference "mlx-audio-swift"',
                        'XCLocalSwiftPackageReference "mlx-audio-swift"')
    lm_pattern = (
        r'\t\tC1D2E3F4A5060708090A0B03 /\* XCRemoteSwiftPackageReference "mlx-swift-lm" \*/ = \{'
        r'.*?\n\t\t\};'
    )
    lm = (
        '\t\tC1D2E3F4A5060708090A0B03 /* XCRemoteSwiftPackageReference "mlx-swift-lm" */ = {\n'
        '\t\t\tisa = XCRemoteSwiftPackageReference;\n'
        '\t\t\trepositoryURL = "https://github.com/ml-explore/mlx-swift-lm.git";\n'
        '\t\t\trequirement = {\n'
        '\t\t\t\tkind = revision;\n'
        f'\t\t\t\trevision = {LM_REVISION};\n'
        '\t\t\t};\n'
        '\t\t};'
    )
    return replace_once(text, lm_pattern, lm)


def prepare(output, audio, root=ROOT):
    output, audio, root = output.resolve(), audio.resolve(), root.resolve()
    if output.exists():
        raise ValueError(f"Output already exists; refusing to overwrite: {output}")
    if output.is_relative_to(root) or output.is_relative_to(audio):
        raise ValueError("Use an output directory outside both source repositories")
    manifest = (audio / "Package.swift").read_text()
    required = ['swift-tools-version:6.3', 'exact: "0.31.6"', f'revision: "{LM_REVISION}"']
    if not all(value in manifest for value in required):
        raise ValueError("The audio checkout does not declare the expected Swift 6.3 / MLX compatibility set")
    project = root / "Voxt.xcodeproj/project.pbxproj"
    converted = candidate_project(project.read_text(), audio)
    output.mkdir(parents=True)
    ignore = shutil.ignore_patterns("xcuserdata", "*.xcuserstate", "Package.resolved", "Signing.local.xcconfig")
    for directory in ["Voxt", "VoxtTests", "Voxt.xcodeproj", "Config"]:
        shutil.copytree(root / directory, output / directory, ignore=ignore)
    (output / "Voxt.xcodeproj/project.pbxproj").write_text(converted)
    # Do not let git discover a repository above the unversioned snapshot.
    env = dict(os.environ, GIT_CEILING_DIRECTORIES=str(output.parent))
    patch = str(root / "tools/mlx-next.patch")
    subprocess.run(["git", "apply", "--check", patch], cwd=output, env=env, check=True)
    subprocess.run(["git", "apply", patch], cwd=output, env=env, check=True)
    (output / "MLX-CANDIDATE.json").write_text(json.dumps({
        "audio_path": str(audio),
        "mlx_swift": "0.31.6",
        "mlx_swift_lm_revision": LM_REVISION,
        "swift_transformers": "1.3.4",
        "swift_huggingface": "0.10.2",
        "status": "unvalidated local candidate; do not release",
    }, indent=2) + "\n")
    return output / "Voxt.xcodeproj"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--audio", type=Path, default=ROOT.parent / "mlx-audio-swift")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    project = prepare(args.output, args.audio)
    print(f"Candidate project: {project}")
    print("Use xcodebuild -resolvePackageDependencies -project <candidate> -scheme Voxt first.")
    print("Then build/test with -onlyUsePackageVersionsFromResolvedFile CODE_SIGNING_ALLOWED=NO.")
    print("The original project and release pins were not changed.")


if __name__ == "__main__":
    main()
