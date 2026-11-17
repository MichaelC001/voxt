package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// machO is a minimal arm64 executable whose one load command is cmd
// (lcBuildVersion: platform, minos; lcVersionMinMacOSX: minos).
func machO(cmd, platform uint32, minos version) []byte {
	le := binary.LittleEndian
	v := uint32(minos.major<<16 | minos.minor<<8 | minos.patch)
	var lc []byte
	switch cmd {
	case lcBuildVersion:
		lc = le.AppendUint32(nil, cmd)
		lc = le.AppendUint32(lc, 24)
		lc = le.AppendUint32(lc, platform)
		lc = le.AppendUint32(lc, v)
		lc = le.AppendUint32(lc, 26<<16) // sdk
		lc = le.AppendUint32(lc, 0)      // ntools
	case lcVersionMinMacOSX:
		lc = le.AppendUint32(nil, cmd)
		lc = le.AppendUint32(lc, 16)
		lc = le.AppendUint32(lc, v)
		lc = le.AppendUint32(lc, 26<<16)
	}
	ncmds := uint32(0)
	if lc != nil {
		ncmds = 1
	}
	b := le.AppendUint32(nil, 0xfeedfacf)
	b = le.AppendUint32(b, 0x0100000c) // arm64
	b = le.AppendUint32(b, 0)
	b = le.AppendUint32(b, 2) // MH_EXECUTE
	b = le.AppendUint32(b, ncmds)
	b = le.AppendUint32(b, uint32(len(lc)))
	b = le.AppendUint32(b, 0)
	b = le.AppendUint32(b, 0)
	return append(b, lc...)
}

// fat wraps thin Mach-O files into a universal one.
func fat(thin ...[]byte) []byte {
	be := binary.BigEndian
	const align = 4096
	b := be.AppendUint32(nil, 0xcafebabe)
	b = be.AppendUint32(b, uint32(len(thin)))
	off := uint32(align)
	for i, t := range thin {
		cpu := uint32(0x0100000c)
		if i == 1 {
			cpu = 0x01000007 // x86_64
			binary.LittleEndian.PutUint32(t[4:], cpu)
		}
		b = be.AppendUint32(b, cpu)
		b = be.AppendUint32(b, 0)
		b = be.AppendUint32(b, off)
		b = be.AppendUint32(b, uint32(len(t)))
		b = be.AppendUint32(b, 12)
		off += align
	}
	for _, t := range thin {
		b = append(b, make([]byte, align-len(b)%align)...)
		b = append(b, t...)
	}
	return b
}

// metallib is a Metal library for macOS (osByte 0x81) target whose
// functions use the Metal language versions langs.
func metallib(osByte byte, target version, langs ...version) []byte {
	le := binary.LittleEndian
	var list []byte
	list = le.AppendUint32(list, uint32(len(langs)))
	for i, l := range langs {
		var e []byte
		name := []byte("kernel_" + string(rune('a'+i)) + "\x00")
		e = append(e, "NAME"...)
		e = le.AppendUint16(e, uint16(len(name)))
		e = append(e, name...)
		e = append(e, "VERS"...)
		e = le.AppendUint16(e, 8)
		for _, x := range []int{2, 8, l.major, l.minor} {
			e = le.AppendUint16(e, uint16(x))
		}
		e = append(e, "ENDT"...)
		list = le.AppendUint32(list, uint32(len(e)+4))
		list = append(list, e...)
	}
	h := []byte("MTLB")
	h = le.AppendUint16(h, 0x8001)
	h = le.AppendUint16(h, 2)
	h = le.AppendUint16(h, 9)
	h = append(h, 0, osByte)
	h = le.AppendUint16(h, uint16(target.major))
	h = le.AppendUint16(h, uint16(target.minor))
	h = le.AppendUint64(h, 0) // file size (unchecked)
	h = le.AppendUint64(h, metallibHeader)
	h = le.AppendUint64(h, uint64(len(list)-4))
	return append(h, list...)
}

func write(t *testing.T, dir, name string, b []byte) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func check(t *testing.T, b []byte) []result {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f")
	write(t, filepath.Dir(p), "f", b)
	res, err := checkFile(p, version{15, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func oks(res []result) []bool {
	var out []bool
	for _, r := range res {
		out = append(out, r.ok)
	}
	return out
}

func TestMachO(t *testing.T) {
	for _, tc := range []struct {
		name string
		file []byte
		want []bool
	}{
		{"build version 15.0", machO(lcBuildVersion, platformMacOS, version{15, 0, 0}), []bool{true}},
		{"build version 14.6.1", machO(lcBuildVersion, platformMacOS, version{14, 6, 1}), []bool{true}},
		{"build version 15.0.1", machO(lcBuildVersion, platformMacOS, version{15, 0, 1}), []bool{false}},
		{"build version 26.2 (issue 162)", machO(lcBuildVersion, platformMacOS, version{26, 2, 0}), []bool{false}},
		{"iOS", machO(lcBuildVersion, 2, version{13, 0, 0}), []bool{false}},
		{"version min 13.0", machO(lcVersionMinMacOSX, 0, version{13, 0, 0}), []bool{true}},
		{"version min 26.0", machO(lcVersionMinMacOSX, 0, version{26, 0, 0}), []bool{false}},
		{"no version", machO(0, 0, version{}), []bool{true}},
		{"universal, both fine", fat(machO(lcBuildVersion, platformMacOS, version{13, 3, 0}), machO(lcBuildVersion, platformMacOS, version{13, 3, 0})), []bool{true, true}},
		{"universal, one too new", fat(machO(lcBuildVersion, platformMacOS, version{13, 3, 0}), machO(lcBuildVersion, platformMacOS, version{26, 0, 0})), []bool{true, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := oks(check(t, tc.file)); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMetallib(t *testing.T) {
	for _, tc := range []struct {
		name string
		file []byte
		want []bool // the target, the language
	}{
		{"MLX's macOS 15 wheel", metallib(0x81, version{15, 0, 0}, version{3, 2, 0}, version{3, 2, 0}), []bool{true, true}},
		{"mlx-swift (macOS 14, Metal 3.1)", metallib(0x81, version{14, 0, 0}, version{3, 1, 0}), []bool{true, true}},
		{"MLX's macOS 26 wheel (issue 162)", metallib(0x81, version{26, 2, 0}, version{4, 0, 0}), []bool{false, false}},
		{"one function in Metal 4", metallib(0x81, version{15, 0, 0}, version{3, 2, 0}, version{4, 0, 0}, version{3, 1, 0}), []bool{true, false}},
		{"a language the table does not know", metallib(0x81, version{15, 0, 0}, version{3, 3, 0}), []bool{true, false}},
		{"iOS", metallib(0x82, version{15, 0, 0}, version{3, 2, 0}), []bool{false, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := oks(check(t, tc.file)); !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBrokenMetallib(t *testing.T) {
	good := metallib(0x81, version{15, 0, 0}, version{3, 2, 0})
	for name, b := range map[string][]byte{
		"short header":     good[:20],
		"cut list":         good[:len(good)-6],
		"no language":      metallib(0x81, version{15, 0, 0}),
		"huge entry count": append(append([]byte{}, good[:metallibHeader]...), 0xff, 0xff, 0, 0),
	} {
		p := filepath.Join(t.TempDir(), "x.metallib")
		write(t, filepath.Dir(p), "x.metallib", b)
		if _, err := checkFile(p, version{15, 0, 0}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "VoxtMini.app", "Contents")
	write(t, app, "MacOS/VoxtMini", machO(lcBuildVersion, platformMacOS, version{13, 0, 0}))
	write(t, app, "Resources/omni/bin/libmlx.dylib", machO(lcBuildVersion, platformMacOS, version{15, 0, 0}))
	write(t, app, "Resources/omni/bin/mlx.metallib", metallib(0x81, version{15, 0, 0}, version{3, 2, 0}))
	write(t, app, "Resources/Info.plist", []byte("<plist/>"))
	var out bytes.Buffer
	if n := run(&out, version{15, 0, 0}, []string{dir}); n != 0 {
		t.Fatalf("%d failures:\n%s", n, out.String())
	}
	if got := strings.Count(out.String(), "ok   "); got != 4 {
		t.Errorf("%d ok lines, want 4:\n%s", got, out.String())
	}
	write(t, app, "Resources/omni/bin/whisper_server", machO(lcBuildVersion, platformMacOS, version{26, 0, 0}))
	out.Reset()
	if n := run(&out, version{15, 0, 0}, []string{dir}); n != 1 || !strings.Contains(out.String(), "FAIL "+filepath.Join(app, "Resources/omni/bin/whisper_server")+": Arm64 needs macOS 26.0, newer than macOS 15.0") {
		t.Errorf("%d failures:\n%s", n, out.String())
	}
	out.Reset()
	if n := run(&out, version{15, 0, 0}, []string{t.TempDir()}); n != 1 {
		t.Errorf("an empty directory passed:\n%s", out.String())
	}
}

func TestMygoMinimum(t *testing.T) {
	v, err := mygoMinimum("../../../mygo.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseVersion(v); err != nil {
		t.Fatal(err)
	}
	if v != "15.0" { // the docs (README, docs/release.md) say macOS 15 too
		t.Errorf("mygo.json's minimum is %s", v)
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string]version{"15": {15, 0, 0}, "15.0": {15, 0, 0}, "26.2.1": {26, 2, 1}} {
		if got, err := parseVersion(in); err != nil || got != want {
			t.Errorf("%s: %v %v", in, got, err)
		}
	}
	for _, in := range []string{"", "x", "15.x", "1.2.3.4", "-1"} {
		if _, err := parseVersion(in); err == nil {
			t.Errorf("%q parsed", in)
		}
	}
}
