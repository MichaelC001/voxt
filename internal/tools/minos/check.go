package main

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// version is a macOS or Metal language version.
type version struct{ major, minor, patch int }

func (v version) String() string {
	if v.patch != 0 {
		return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
	}
	return fmt.Sprintf("%d.%d", v.major, v.minor)
}

func (v version) newerThan(o version) bool {
	if v.major != o.major {
		return v.major > o.major
	}
	if v.minor != o.minor {
		return v.minor > o.minor
	}
	return v.patch > o.patch
}

func parseVersion(s string) (version, error) {
	var v version
	parts := strings.Split(s, ".")
	if len(parts) > 3 {
		return v, fmt.Errorf("not a version: %q", s)
	}
	n := []*int{&v.major, &v.minor, &v.patch}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return v, fmt.Errorf("not a version: %q", s)
		}
		*n[i] = x
	}
	return v, nil
}

// packed is a Mach-O version: xxxx.yy.zz in nibbles.
func packed(x uint32) version {
	return version{int(x >> 16), int(x >> 8 & 0xff), int(x & 0xff)}
}

// result is one thing checked in a file: an architecture of a Mach-O
// file, or a Metal library.
type result struct {
	what string
	ok   bool
}

// checkFile checks path if it is a Mach-O file or a Metal library (nil
// results for any other file).
func checkFile(path string, limit version) ([]result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return nil, nil // shorter than any magic
	}
	switch {
	case string(magic[:]) == "MTLB":
		return checkMetallib(f, limit)
	case isMachO(magic):
		return checkMachO(f, limit)
	}
	return nil, nil
}

func isMachO(m [4]byte) bool {
	le, be := binary.LittleEndian.Uint32(m[:]), binary.BigEndian.Uint32(m[:])
	for _, x := range []uint32{macho.Magic32, macho.Magic64} {
		if le == x || be == x {
			return true
		}
	}
	return be == macho.MagicFat
}

// Mach-O load commands that carry the oldest macOS a file runs on.
const (
	lcVersionMinMacOSX = 0x24
	lcBuildVersion     = 0x32
	platformMacOS      = 1
)

func checkMachO(r io.ReaderAt, limit version) ([]result, error) {
	var files []*macho.File
	if fat, err := macho.NewFatFile(r); err == nil {
		for _, a := range fat.Arches {
			files = append(files, a.File)
		}
	} else if errors.Is(err, macho.ErrNotFat) {
		f, err := macho.NewFile(r)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	} else {
		return nil, err
	}
	var out []result
	for _, f := range files {
		out = append(out, machOMinimum(f, limit))
	}
	return out, nil
}

// machOMinimum checks one architecture's minos.
func machOMinimum(f *macho.File, limit version) result {
	arch := strings.TrimPrefix(f.Cpu.String(), "Cpu")
	for _, l := range f.Loads {
		raw := l.Raw()
		if len(raw) < 16 {
			continue
		}
		bo := f.ByteOrder
		switch bo.Uint32(raw) {
		case lcBuildVersion:
			if p := bo.Uint32(raw[8:]); p != platformMacOS {
				return result{fmt.Sprintf("%s built for platform %d, not macOS", arch, p), false}
			}
			v := packed(bo.Uint32(raw[12:]))
			return result{fmt.Sprintf("%s needs macOS %s", arch, v), !v.newerThan(limit)}
		case lcVersionMinMacOSX:
			v := packed(bo.Uint32(raw[8:]))
			return result{fmt.Sprintf("%s needs macOS %s", arch, v), !v.newerThan(limit)}
		}
	}
	return result{arch + " names no minimum macOS", true}
}

// A Metal library's header (all little-endian): "MTLB", the target
// platform (u16), the file format's version (u16 major, u16 minor), the
// file type (u8), the operating system it targets (u8: 0x81 macOS) and
// that system's version (u16 major, u16 minor), the file's size (u64), the
// function list's offset and size (u64 each), then other sections. The
// function list is a count (u32, outside the size) and an entry per function: its size
// (u32, itself included) and tags, a four-letter name and a u16 length
// before the data, up to "ENDT". "VERS" holds the AIR version and the
// Metal language version (u16 major, minor each).
const metallibHeader = 40

// osMacOS is the low bits of the header's operating system byte.
const osMacOS = 1

func checkMetallib(r io.ReaderAt, limit version) ([]result, error) {
	h := make([]byte, metallibHeader)
	if _, err := r.ReadAt(h, 0); err != nil {
		return nil, fmt.Errorf("a short Metal library header: %w", err)
	}
	le := binary.LittleEndian
	osByte := h[11]
	target := version{major: int(le.Uint16(h[12:])), minor: int(le.Uint16(h[14:]))}
	listOff, listSize := le.Uint64(h[24:]), le.Uint64(h[32:])
	if listSize > 1<<30 {
		return nil, fmt.Errorf("a Metal library with an unreadable function list")
	}
	list := make([]byte, listSize+4) // the size leaves out the count
	if _, err := r.ReadAt(list, int64(listOff)); err != nil {
		return nil, fmt.Errorf("a Metal library with a short function list: %w", err)
	}
	lang, n, err := metalLanguage(list)
	if err != nil {
		return nil, err
	}
	var out []result
	if osByte&0x7f != osMacOS {
		out = append(out, result{fmt.Sprintf("Metal library for operating system 0x%02x, not macOS", osByte), false})
	} else {
		out = append(out, result{fmt.Sprintf("Metal library targets macOS %s", target), !target.newerThan(limit)})
	}
	need := languageMacOS(lang)
	out = append(out, result{fmt.Sprintf("Metal language %s (%d functions) needs macOS %s", lang, n, need), !need.newerThan(limit)})
	return out, nil
}

// metalLanguage is the newest Metal language version among the
// functions in a function list, and their number.
func metalLanguage(list []byte) (version, int, error) {
	le := binary.LittleEndian
	bad := errors.New("an unreadable Metal function list")
	if len(list) < 4 {
		return version{}, 0, bad
	}
	count := int(le.Uint32(list))
	var newest version
	seen := false
	p := list[4:]
	for i := 0; i < count; i++ {
		if len(p) < 4 {
			return version{}, 0, bad
		}
		size := int(le.Uint32(p))
		if size < 4 || size > len(p) {
			return version{}, 0, bad
		}
		entry := p[4:size]
		p = p[size:]
		for len(entry) >= 4 {
			tag := entry[:4]
			entry = entry[4:]
			if bytes.Equal(tag, []byte("ENDT")) {
				break
			}
			if len(entry) < 2 {
				return version{}, 0, bad
			}
			l := int(le.Uint16(entry))
			entry = entry[2:]
			if l > len(entry) {
				return version{}, 0, bad
			}
			data := entry[:l]
			entry = entry[l:]
			if bytes.Equal(tag, []byte("VERS")) && l >= 8 {
				v := version{major: int(le.Uint16(data[4:])), minor: int(le.Uint16(data[6:]))}
				if !seen || v.newerThan(newest) {
					newest, seen = v, true
				}
			}
		}
	}
	if !seen {
		return version{}, 0, errors.New("a Metal library without a language version")
	}
	return newest, count, nil
}

// languageMacOS is the oldest macOS whose Metal runs a Metal language
// version (Apple's Metal Shading Language specification; an older
// language counts as macOS 11's): a version newer than the table knows is
// assumed to need a macOS newer than any.
func languageMacOS(lang version) version {
	table := []struct{ lang, macOS version }{
		{version{2, 3, 0}, version{11, 0, 0}},
		{version{2, 4, 0}, version{12, 0, 0}},
		{version{3, 0, 0}, version{13, 0, 0}},
		{version{3, 1, 0}, version{14, 0, 0}},
		{version{3, 2, 0}, version{15, 0, 0}},
		{version{4, 0, 0}, version{26, 0, 0}},
	}
	for _, t := range table {
		if !lang.newerThan(t.lang) {
			return t.macOS
		}
	}
	return version{99, 0, 0}
}
