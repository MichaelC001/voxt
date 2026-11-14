package vad

import "slices"

// Test-only views of the detector's state.

// Heard reports whether speech started at least once.
func (d *Detector) Heard() bool { return d.first >= 0 }

// Segments are the closed segments, plus the open one ended at the current
// sample.
func (d *Detector) Segments() []Segment {
	s := slices.Clone(d.segs)
	if d.speaking {
		s = append(s, Segment{d.segStart, d.pos})
	}
	return s
}
