package model

import (
	"bytes"
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Settings are the addon's own settings, a file of the tree like the programs: settings.yaml.
type Settings struct {
	// Location is where the system stands, for the astro time modules; nil until it is set.
	Location *Location `json:"location,omitempty" yaml:"location,omitempty"`
	// PacingMS is the pause between two write commands on a radio interface, in milliseconds
	// (0: the default, DefaultPacingMS).
	PacingMS int `json:"pacing_ms,omitempty" yaml:"pacing_ms,omitempty"`
	// LoopGuard pauses a program that runs too often; nil: the defaults.
	LoopGuard *LoopGuard `json:"loop_guard,omitempty" yaml:"loop_guard,omitempty"`
}

// LoopGuard is the loop guard's setting: a program that runs more often than Runs times within
// WindowSeconds is paused for PauseSeconds. A zero field is its default.
type LoopGuard struct {
	Runs          int `json:"runs,omitempty" yaml:"runs,omitempty"`
	WindowSeconds int `json:"window_seconds,omitempty" yaml:"window_seconds,omitempty"`
	PauseSeconds  int `json:"pause_seconds,omitempty" yaml:"pause_seconds,omitempty"`
}

// The defaults of the settings (concept OQ-9), and their limits.
const (
	DefaultPacingMS   = 300
	MaxPacingMS       = 10000
	DefaultLoopRuns   = 20
	DefaultLoopWindow = 10
	DefaultLoopPause  = 60
)

// Location is a position in degrees: north and east are positive.
type Location struct {
	Latitude  float64 `json:"latitude" yaml:"latitude"`
	Longitude float64 `json:"longitude" yaml:"longitude"`
}

// SettingsPath is the settings' file in the tree.
const SettingsPath = "settings.yaml"

// Check says what is wrong with the settings.
func (s *Settings) Check() error {
	if l := s.Location; l != nil {
		if !(l.Latitude >= -90 && l.Latitude <= 90) {
			return fmt.Errorf("location: the latitude is between -90 and 90 degrees")
		}
		if !(l.Longitude >= -180 && l.Longitude <= 180) {
			return fmt.Errorf("location: the longitude is between -180 and 180 degrees")
		}
	}
	if s.PacingMS != 0 && (s.PacingMS < 100 || s.PacingMS > MaxPacingMS) {
		return fmt.Errorf("pacing_ms: the pause between write commands is between 100 and %d milliseconds", MaxPacingMS)
	}
	if g := s.LoopGuard; g != nil {
		if g.Runs != 0 && (g.Runs < 2 || g.Runs > 1000) {
			return fmt.Errorf("loop_guard: runs is between 2 and 1000")
		}
		if g.WindowSeconds != 0 && (g.WindowSeconds < 1 || g.WindowSeconds > 600) {
			return fmt.Errorf("loop_guard: window_seconds is between 1 and 600")
		}
		if g.PauseSeconds != 0 && (g.PauseSeconds < 1 || g.PauseSeconds > 86400) {
			return fmt.Errorf("loop_guard: pause_seconds is between 1 and 86400")
		}
	}
	return nil
}

// Pacing answers the pause between write commands on a radio interface in milliseconds: the
// setting, or the default.
func (s *Settings) Pacing() int {
	if s.PacingMS == 0 {
		return DefaultPacingMS
	}
	return s.PacingMS
}

// Guard answers the loop guard with the defaults filled in.
func (s *Settings) Guard() LoopGuard {
	g := LoopGuard{}
	if s.LoopGuard != nil {
		g = *s.LoopGuard
	}
	if g.Runs == 0 {
		g.Runs = DefaultLoopRuns
	}
	if g.WindowSeconds == 0 {
		g.WindowSeconds = DefaultLoopWindow
	}
	if g.PauseSeconds == 0 {
		g.PauseSeconds = DefaultLoopPause
	}
	return g
}

// EncodeSettings writes the settings the one way their file has them.
func EncodeSettings(s *Settings) []byte {
	var b bytes.Buffer
	b.WriteString("# The addon's settings.\n")
	if l := s.Location; l != nil {
		b.WriteString("# location: where the system stands, in degrees (north and east positive) - for sunrise and sunset\n")
		fmt.Fprintf(&b, "location:\n  latitude: %s\n  longitude: %s\n", strconv.FormatFloat(l.Latitude, 'f', -1, 64), strconv.FormatFloat(l.Longitude, 'f', -1, 64))
	}
	if s.PacingMS != 0 {
		b.WriteString("# pacing_ms: the pause between two write commands on a radio interface, in milliseconds\n")
		fmt.Fprintf(&b, "pacing_ms: %d\n", s.PacingMS)
	}
	if g := s.LoopGuard; g != nil && *g != (LoopGuard{}) {
		b.WriteString("# loop_guard: a program that runs more often than \"runs\" times within the window is paused\n")
		b.WriteString("loop_guard:\n")
		for _, f := range []struct {
			key string
			v   int
		}{{"runs", g.Runs}, {"window_seconds", g.WindowSeconds}, {"pause_seconds", g.PauseSeconds}} {
			if f.v != 0 {
				fmt.Fprintf(&b, "  %s: %d\n", f.key, f.v)
			}
		}
	}
	return b.Bytes()
}

// DecodeSettings reads a settings file; an empty one is no settings.
func DecodeSettings(data []byte) (*Settings, error) {
	s := &Settings{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(s); err != nil && len(bytes.TrimSpace(data)) > 0 && err.Error() != "EOF" {
		return nil, fmt.Errorf("%s: %v", SettingsPath, err)
	}
	if err := s.Check(); err != nil {
		return nil, fmt.Errorf("%s: %v", SettingsPath, err)
	}
	return s, nil
}
