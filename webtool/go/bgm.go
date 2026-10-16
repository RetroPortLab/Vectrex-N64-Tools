package main

// bgmtool as a library: an mp3 (bytes) of the soundtrack -> the cartridge's BGMnn.WAV (bytes). Same code as tools/bgmtool.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"strings"

	mp3 "github.com/hajimehoshi/go-mp3"
)

type track struct {
	nn   int
	loop bool
	cut  float64 // seconds to keep (0: all): the hang glider recording holds its tune twice, and the track is one pass
}

// Pilotwings 64's soundtrack: the file's number and name -> the game's track number (the file number minus one); the flight themes and menus loop, the jingles play once
var pw = map[string]track{
	"01 opening_theme": {0, false, 0}, "02 title_demo": {1, true, 0}, "03 game_menu": {2, true, 0}, "04 mission_menu": {3, true, 0},
	"05 hang_glider": {4, true, 154.6955}, "06 hang_glider_-_landing_accomplished": {5, false, 0}, "07 hang_glider_-_landed_outside_target": {6, false, 0}, "08 hang_glider_-_crash": {7, false, 0},
	"09 rocket_belt": {8, true, 0}, "10 rocket_belt_-_landing_accomplished": {9, false, 0}, "11 rocket_belt_-_landed_outside_target": {10, false, 0}, "12 rocket_belt_-_crash": {11, false, 0},
	"13 gyrocopter": {12, true, 0}, "14 gyrocopter_-_landing_accomplished": {13, false, 0}, "15 gyrocopter_-_landed_outside_target": {14, false, 0}, "16 gyrocopter_-_crash": {15, false, 0},
	"17 cannonball": {16, true, 0}, "18 cannonball_-_hit_the_target": {17, false, 0}, "19 cannonball_-_missed_the_target": {18, false, 0},
	"20 sky_diving": {19, true, 0}, "21 sky_diving_-_landing_accomplished": {20, false, 0}, "22 sky_diving_-_landed_outside_target": {21, false, 0}, "23 sky_diving_-_accident": {22, false, 0},
	"24 jumble_hopper": {23, true, 0}, "25 jumble_hopper_-_reached_goal_area": {24, false, 0},
	"26 birdman": {25, true, 0}, "27 birdman_-_reached_goal_area_unused": {26, false, 0}, "28 birdman_-_crash": {27, false, 0},
	"29 replay": {28, true, 0}, "31 bravissimo_credits": {29, true, 0},
}

// Mario Kart 64's: loops from the start unless it is a fanfare
var mk = map[string]track{
	"01 title_screen": {0, true, 0}, "02 main_menu": {1, true, 0}, "03 game_menu": {2, true, 0}, "03 race_start_fanfare": {3, false, 0},
	"04 luigi_s_raceway_-_mario_raceway_-_royal_raceway_-_wario_stadium": {4, true, 0}, "05 moo_moo_farm_-_yoshi_valley": {5, true, 0}, "06 koopa_troopa_beach": {6, true, 0},
	"07 kalimari_desert": {7, true, 0}, "08 toads_turnpike": {8, true, 0}, "09 frappe_snowland_-_sherbet_land": {9, true, 0}, "10 choco_mountain": {10, true, 0}, "11 bowsers_castle": {11, true, 0},
	"12 dks_jungle_parkway": {12, true, 0}, "13 banshee_boardwalk": {13, true, 0}, "14 rainbow_road": {14, true, 0}, "15 super_star": {15, true, 0}, "16 final_lap_fanfare": {16, false, 0},
	"18 2nd-4th_place_fanfare": {17, false, 0}, "19 1st-4th_place_results": {18, true, 0}, "31 bravissimo_credits": {19, true, 0},
}

var (
	nameRe   = regexp.MustCompile(`^(\d+)\.\s*_?\s*(.*)$`)
	dupRe    = regexp.MustCompile(`(\s*\(\d+\)|_\d)$`)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
)

// squash makes "05._Hang_Glider" and "05. Hang Glider" and "Bravissimo! (Credits)" comparable: lower case letters and digits only
func squash(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

// squashed copies of the tables, filled in at start
var pwS, mkS map[string]track

func squashTable(t map[string]track) map[string]track {
	o := map[string]track{}
	for k, v := range t {
		o[squash(k)] = v
	}
	return o
}

// key makes a file name ("05._Hang_Glider.mp3", "05. Hang Glider.mp3", "abc123de-05._Hang_Glider_1.mp3") into the comparable "05hangglider"
func key(path string) string {
	b := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	b = regexp.MustCompile(`(?i)^[0-9a-f]{8}-`).ReplaceAllString(b, "") // a download prefix
	m := nameRe.FindStringSubmatch(b)
	if m == nil {
		return ""
	}
	n := dupRe.ReplaceAllString(strings.TrimSpace(m[2]), "") // "(1)" or "_1": a second copy
	return fmt.Sprintf("%02s", m[1]) + squash(n)
}

// decode reads an mp3 into mono float samples at its own rate
func decode(data []byte) ([]float64, int, error) {
	d, err := mp3.NewDecoder(bytes.NewReader(data))
	if err != nil {
		return nil, 0, err
	}
	raw, err := io.ReadAll(d)
	if err != nil {
		return nil, 0, err
	}
	n := len(raw) / 4
	x := make([]float64, n)
	for i := 0; i < n; i++ {
		l := float64(int16(binary.LittleEndian.Uint16(raw[i*4:])))
		r := float64(int16(binary.LittleEndian.Uint16(raw[i*4+2:])))
		x[i] = (l + r) / 2
	}
	if len(x) > 4000 {
		x = x[2257:] // the encoder and decoder delay at the start (measured against ffmpeg's output on 44.1 kHz files: 51 ms)
	}
	return x, d.SampleRate(), nil
}

// resample to 32 kHz with a windowed-sinc low-pass (cut-off 15 kHz) so nothing folds back
func resample(x []float64, from int) []float64 {
	const to, half, res = 32000, 24, 256
	if from == to {
		return x
	}
	ratio := float64(from) / to
	cut := math.Min(1.0, 1.0/ratio) * 15000.0 / 16000.0
	tab := make([]float64, 2*half*res+1) // the kernel at 1/256-sample steps
	for i := range tab {
		t := (float64(i)/res - half)
		u := t * cut
		sinc := 1.0
		if u != 0 {
			sinc = math.Sin(math.Pi*u) / (math.Pi * u)
		}
		tab[i] = sinc * (0.5 + 0.5*math.Cos(math.Pi*t/half))
	}
	reach := int(math.Ceil(half / cut))
	out := make([]float64, int(float64(len(x))/ratio))
	for i := range out {
		c := float64(i) * ratio
		var s, w float64
		for j := int(c) - reach; j <= int(c)+reach; j++ {
			if j < 0 || j >= len(x) {
				continue
			}
			t := (float64(j) - c) * cut
			if t <= -half || t >= half {
				continue
			}
			k := tab[int((t+half)*res+0.5)]
			s += x[j] * k
			w += k
		}
		if w != 0 {
			out[i] = s / w
		}
	}
	return out
}

func makeWav(x []float64, loop bool) []byte {
	pk := 0.0
	for _, v := range x {
		pk = math.Max(pk, math.Abs(v))
	}
	if pk == 0 {
		pk = 1
	}
	g := math.Min(0.9, 32767.0/pk)
	pcm := make([]byte, len(x)*2)
	for i, v := range x {
		s := math.Max(-32768, math.Min(32767, math.Round(v*g)))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(s)))
	}
	lp := uint32(0xFFFFFFFF)
	if loop {
		lp = 0
	}
	var b []byte
	le := binary.LittleEndian
	b = append(b, "RIFF"...)
	b = le.AppendUint32(b, uint32(4+24+12+8+len(pcm)))
	b = append(b, "WAVEfmt "...)
	b = le.AppendUint32(b, 16)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint32(b, 32000)
	b = le.AppendUint32(b, 64000)
	b = le.AppendUint16(b, 2)
	b = le.AppendUint16(b, 16)
	b = append(b, "loop"...)
	b = le.AppendUint32(b, 4)
	b = le.AppendUint32(b, lp)
	b = append(b, "data"...)
	b = le.AppendUint32(b, uint32(len(pcm)))
	b = append(b, pcm...)
	return b
}

func init() { pwS, mkS = squashTable(pw), squashTable(mk) }

// convertMP3 turns one soundtrack mp3 into its track: the track number, a name, the WAV; known=false when the file name is not one of the soundtrack's
func convertMP3(game, name string, data []byte) (nn int, wav []byte, secs float64, loops, known bool, err error) {
	table := pwS
	if game == "mk" {
		table = mkS
	}
	t, ok := table[key(name)]
	if !ok {
		return 0, nil, 0, false, false, nil
	}
	x, rate, err := decode(data)
	if err != nil {
		return t.nn, nil, 0, t.loop, true, err
	}
	x = resample(x, rate)
	if t.cut > 0 && int(t.cut*32000) < len(x) {
		x = x[:int(t.cut*32000)]
	}
	return t.nn, makeWav(x, t.loop), float64(len(x)) / 32000, t.loop, true, nil
}
