package main

// The Pilotwings 64 ROM converter as a library: the same code as tools/pw64tool, with bytes in and files out (no disk), for the web page.

import (
	"fmt"
	"strings"
)

const bgmCount = 30

// tracks 0-15 are recordings of the original; only 16 up come from the ROM's sequences
const bgmRecorded = 16

var bgmLoops = map[int]bool{1: true, 2: true, 3: true, 4: true, 8: true, 12: true, 16: true, 19: true, 23: true, 25: true, 28: true, 29: true}

type outFile struct {
	Name string
	Data []byte
}

// toZ64 accepts the three byte orders ROMs come in and returns the big-endian one (.z64).
func toZ64(b []byte) ([]byte, string, error) {
	if len(b) < 0x1000 {
		return nil, "", fmt.Errorf("the file is too small to be an N64 ROM")
	}
	out := append([]byte{}, b...)
	switch {
	case b[0] == 0x80 && b[1] == 0x37:
		return out, "z64", nil
	case b[0] == 0x37 && b[1] == 0x80:
		for i := 0; i+1 < len(out); i += 2 {
			out[i], out[i+1] = out[i+1], out[i]
		}
		return out, "v64 (byte swapped)", nil
	case b[0] == 0x40 && b[1] == 0x12:
		for i := 0; i+3 < len(out); i += 4 {
			out[i], out[i+1], out[i+2], out[i+3] = out[i+3], out[i+2], out[i+1], out[i]
		}
		return out, "n64 (little endian)", nil
	}
	return nil, "", fmt.Errorf("this does not look like an N64 ROM (unknown first bytes)")
}

// convertROM makes ISL0-3.BIN, TASKS.BIN, TEXT.BIN, SFX.BIN and the music; say reports progress.
func convertROM(raw []byte, allMusic, noMusic bool, say func(string)) ([]outFile, error) {
	z, order, err := toZ64(raw)
	if err != nil {
		return nil, err
	}
	if len(z) < 0x40 || string(z[0x3B:0x3F]) != "NPWE" {
		return nil, fmt.Errorf("this is not the USA version of Pilotwings 64 (game code %q, expected NPWE)", string(z[0x3B:0x3F]))
	}
	say(fmt.Sprintf("ROM read: %s, %d MB, Pilotwings 64 (USA)", order, len(z)>>20))
	files, err := parseFS(z)
	if err != nil || len(files) < 100 {
		return nil, fmt.Errorf("the ROM's file system could not be read (%v): is this an unmodified USA ROM?", err)
	}
	r := &rom{files}
	var out []outFile
	step := func(what string, f func() ([]byte, string, error), name string) error {
		d, msg, err := f()
		if err != nil {
			return fmt.Errorf("%s: %v", what, err)
		}
		out = append(out, outFile{name, d})
		say(fmt.Sprintf("%s: %s (%s, %d KB)", what, name, msg, len(d)/1024))
		return nil
	}
	for i := 0; i < 4; i++ {
		i := i
		if err := step(islandNames[i], func() ([]byte, string, error) { return r.exportIsland(i) }, fmt.Sprintf("ISL%d.BIN", i)); err != nil {
			return nil, err
		}
	}
	for _, s := range []struct {
		what, name string
		f          func() ([]byte, string, error)
	}{{"Tests", "TASKS.BIN", r.exportTasks}, {"Test words", "TEXT.BIN", r.exportText}, {"Sound effects", "SFX.BIN", r.exportSFX}} {
		if err := step(s.what, s.f, s.name); err != nil {
			return nil, err
		}
	}
	if !noMusic {
		first := bgmRecorded
		if allMusic {
			first = 0
		}
		mc, err := newMusic(r, z)
		if err != nil {
			return nil, fmt.Errorf("music: %v", err)
		}
		for n := first; n < bgmCount; n++ {
			d, msg, err := mc.render(n, bgmLoops[n])
			name := fmt.Sprintf("BGM%02d.WAV", n)
			if err != nil {
				say(fmt.Sprintf("%s skipped: %v", name, err))
				continue
			}
			out = append(out, outFile{name, d})
			say(fmt.Sprintf("%s: %s", name, strings.TrimSpace(msg)))
		}
	}
	return out, nil
}
