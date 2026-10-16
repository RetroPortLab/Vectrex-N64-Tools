package main

// The music: renders a sequence of the ROM (libultra "compressed MIDI") with the ROM's own sound bank to 32 kHz mono, as the game's BGMnn.WAV (ported from tools/render_pw64_music.py,
// which documents the method: mono, no reverb or pan, linear interpolation, envelope times as stored).

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

type trk struct {
	sq     []byte
	p      int
	stack  [][2]int
	bu, bl int
	run    int
	err    error
}

func (t *trk) raw() (int, int) {
	for t.bl == 0 && len(t.stack) > 0 {
		e := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		t.bu, t.bl = e[0], e[1]
	}
	var pos int
	if t.bl > 0 {
		pos = t.bu
		t.bu++
		t.bl--
	} else {
		pos = t.p
		t.p++
	}
	if pos < 0 || pos >= len(t.sq) {
		t.err = fmt.Errorf("a sequence runs past its end")
		return 0xFF, 0
	}
	return int(t.sq[pos]), pos
}
func (t *trk) byte() int {
	b, pos := t.raw()
	for b == 0xFE && t.err == nil {
		hi, _ := t.raw()
		if hi == 0xFE {
			return 0xFE
		}
		lo, _ := t.raw()
		ln, _ := t.raw()
		if t.bl > 0 {
			t.stack = append(t.stack, [2]int{t.bu, t.bl})
		}
		t.bu = pos - (hi<<7<<1 | lo)
		t.bl = ln
		b, pos = t.raw()
	}
	return b
}
func (t *trk) varlen() int {
	b := t.byte()
	v := b & 0x7f
	for b&0x80 != 0 && t.err == nil {
		b = t.byte()
		v = v<<7 | b&0x7f
	}
	return v
}

type sev struct {
	t, kind     int // kind: 0 tempo, 1 note, 2 cc, 3 prog, 4 bend
	ch, a, b, c int // note: key, vel, dur; cc: number, value; prog: program; bend: value
}

func decodeTrack(sq []byte, start int) ([]sev, int, error) {
	T := &trk{sq: sq, p: start, run: -1}
	t := 0
	var out []sev
	for {
		t += T.varlen()
		b := T.byte()
		first := -1
		if b&0x80 == 0 {
			first = b
			b = T.run
		} else {
			T.run = b
		}
		if T.err != nil || b < 0 {
			return nil, 0, fmt.Errorf("a sequence is damaged")
		}
		nxt := func() int {
			if first >= 0 {
				x := first
				first = -1
				return x
			}
			return T.byte()
		}
		hi := b & 0xF0
		switch {
		case b == 0xFF:
			ty := nxt()
			switch ty {
			case 0x51:
				x := T.byte() << 16
				x |= T.byte() << 8
				x |= T.byte()
				out = append(out, sev{t: t, kind: 0, a: x})
			case 0x2F:
				return out, t, nil
			case 0x2E:
				T.byte()
				T.byte()
			case 0x2D:
				for i := 0; i < 6; i++ {
					T.byte()
				}
			default:
				return nil, 0, fmt.Errorf("unknown meta event %02x", ty)
			}
		case hi == 0x90:
			k := nxt()
			v := T.byte()
			d := T.varlen()
			out = append(out, sev{t: t, kind: 1, ch: b & 15, a: k, b: v, c: d})
		case hi == 0xB0:
			c := nxt()
			v := T.byte()
			out = append(out, sev{t: t, kind: 2, ch: b & 15, a: c, b: v})
		case hi == 0xC0:
			out = append(out, sev{t: t, kind: 3, ch: b & 15, a: nxt()})
		case hi == 0xE0:
			lo := nxt()
			h2 := T.byte()
			out = append(out, sev{t: t, kind: 4, ch: b & 15, a: (h2<<7 | lo) - 8192})
		default:
			return nil, 0, fmt.Errorf("unknown status %02x", b)
		}
		if T.err != nil {
			return nil, 0, T.err
		}
	}
}

func findSeqFile(rom []byte) (int, [][2]int, error) {
	pat := []byte{0x53, 0x31, 0x00, 0x1f}
	for o := 0; o+4+8*31 <= len(rom); o++ {
		if rom[o] != pat[0] || rom[o+1] != pat[1] || rom[o+2] != pat[2] || rom[o+3] != pat[3] {
			continue
		}
		ents := make([][2]int, 31)
		ok := true
		for i := range ents {
			ents[i] = [2]int{be32(rom[o+4+8*i:]), be32(rom[o+8+8*i:])}
			if !(ents[i][1] > 0 && ents[i][1] < 0x20000) {
				ok = false
				break
			}
		}
		if ok {
			return o, ents, nil
		}
	}
	return 0, nil, fmt.Errorf("the music sequences were not found: is this the USA ROM?")
}

type musicCtx struct {
	rom   []byte
	bk    *bank
	base  int
	ents  [][2]int
	cache map[int][]sound
}

func newMusic(r *rom, raw []byte) (*musicCtx, error) {
	bk, err := newBank(r)
	if err != nil {
		return nil, err
	}
	base, ents, err := findSeqFile(raw)
	if err != nil {
		return nil, err
	}
	m := &musicCtx{rom: raw, bk: bk, base: base, ents: ents, cache: map[int][]sound{}}
	for p := 0; p < bk.nInst; p++ {
		m.cache[p] = bk.instrument(p)
	}
	return m, nil
}

type chanSt struct{ prog, vol, expr, bend int }

// render makes the WAV file of BGM number n; loops: the sequence's second pass is kept (the tail of the first sounds into it), else it plays once.
func (m *musicCtx) render(n int, loops bool) ([]byte, string, error) {
	if n >= len(m.ents) {
		return nil, "", fmt.Errorf("no sequence %d", n)
	}
	o, l := m.ents[n][0], m.ents[n][1]
	if m.base+o+l > len(m.rom) {
		return nil, "", fmt.Errorf("sequence %d is cut off", n)
	}
	sq := m.rom[m.base+o : m.base+o+l]
	var offs [17]int
	for i := range offs {
		offs[i] = be32(sq[4*i:])
	}
	div := offs[16]
	var events []sev
	total := 0
	for _, x := range offs[:16] {
		if x == 0 {
			continue
		}
		ev, t, err := decodeTrack(sq, x)
		if err != nil {
			return nil, "", err
		}
		events = append(events, ev...)
		total = max(total, t)
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].t < events[j].t })
	type tm struct{ t, us int }
	tempo := []tm{}
	for _, e := range events {
		if e.kind == 0 {
			tempo = append(tempo, tm{e.t, e.a})
		}
	}
	tickToSec := func(tk int) float64 {
		sec := 0.0
		lastT, lastUs := 0, 500000
		for _, x := range tempo {
			if x.t >= tk {
				break
			}
			sec += float64(x.t-lastT) / float64(div) * float64(lastUs) / 1e6
			lastT, lastUs = x.t, x.us
		}
		return sec + float64(tk-lastT)/float64(div)*float64(lastUs)/1e6
	}
	passes := 1
	loopN := 0
	if loops {
		passes, loopN = 2, 1
	}
	durPass := tickToSec(total)
	N := int(durPass*sr*float64(passes)) + sr
	buf := make([]float64, N)
	bankRate := float64(m.bk.rate)
	for ps := 0; ps < passes; ps++ {
		offset := total * ps
		var ch [16]chanSt
		for i := range ch {
			ch[i] = chanSt{0, 100, 127, 0}
		}
		for _, e := range events {
			switch e.kind {
			case 3:
				ch[e.ch].prog = e.a
			case 4:
				ch[e.ch].bend = e.a
			case 2:
				if e.a == 7 {
					ch[e.ch].vol = e.b
				} else if e.a == 11 {
					ch[e.ch].expr = e.b
				}
			case 1:
				c := ch[e.ch]
				sounds := m.cache[c.prog]
				if len(sounds) == 0 {
					continue
				}
				s := &sounds[0]
				for i := range sounds {
					q := &sounds[i]
					if q.kmin <= e.a && e.a <= q.kmax && q.vmin <= e.b && e.b <= q.vmax {
						s = q
						break
					}
				}
				t0 := int(tickToSec(e.t+offset) * sr)
				t1 := int(tickToSec(e.t+e.c+offset) * sr)
				semis := float64(e.a-s.kbase) + float64(s.det)/100.0 + float64(c.bend)/8192.0*2.0
				step := (bankRate / sr) * math.Pow(2.0, semis/12.0)
				fv, fc, fe := float64(e.b)/127.0, float64(c.vol)/127.0, float64(c.expr)/127.0
				gain := fv * fv * fc * fc * fe * fe * s.svol * s.vol * 1.4
				pcm := s.pcm
				nHold := max(1, t1-t0)
				relN := max(int(s.rel*sr), int(0.004*sr))
				nTotal := nHold + relN
				if !s.loop {
					nTotal = min(nTotal, int(float64(len(pcm)-1)/step))
				}
				if nTotal <= 0 || t0 >= len(buf) {
					continue
				}
				end := min(len(buf), t0+nTotal)
				a := math.Max(s.atk, 0.002)
				span := float64(max(1, s.le-s.ls))
				for k := 0; k < end-t0; k++ {
					p := float64(k) * step
					if s.loop && p >= float64(s.le) {
						p = float64(s.ls) + math.Mod(p-float64(s.ls), span)
					}
					i0 := min(int(p), len(pcm)-2)
					fr := p - float64(i0)
					x := pcm[i0]*(1-fr) + pcm[i0+1]*fr
					tt := float64(k) / sr
					env := math.Min(1.0, tt/a)
					if s.dec > 0 && s.dv < 1.0 && tt > a {
						env = math.Max(s.dv, 1.0-(1.0-s.dv)*(tt-a)/s.dec)
					}
					rel := 1.0
					if k >= nHold {
						rel = math.Max(0, math.Min(1, 1.0-float64(k-nHold)/float64(relN)))
					}
					buf[t0+k] += x * env * rel * gain
				}
			}
		}
	}
	if loops {
		a := int(durPass * sr * float64(loopN))
		buf = buf[a : a+int(durPass*sr)]
	} else {
		buf = buf[:int(durPass*sr)+sr]
	}
	peak := 0.0
	for _, v := range buf {
		peak = math.Max(peak, math.Abs(v))
	}
	peak += 1e-9
	g := 0.85 / peak
	pcm16 := make([]byte, 0, len(buf)*2)
	for _, v := range buf {
		x := math.Max(-32768, math.Min(32767, v*g*32767))
		pcm16 = binary.LittleEndian.AppendUint16(pcm16, uint16(int16(int(x))))
	}
	loopVal := uint32(0xFFFFFFFF)
	if loops {
		loopVal = 0
	}
	nb := len(pcm16)
	w := []byte("RIFF")
	w = binary.LittleEndian.AppendUint32(w, uint32(4+24+12+8+nb))
	w = append(w, "WAVE"...)
	w = append(w, "fmt "...)
	w = binary.LittleEndian.AppendUint32(w, 16)
	w = binary.LittleEndian.AppendUint16(w, 1)
	w = binary.LittleEndian.AppendUint16(w, 1)
	w = binary.LittleEndian.AppendUint32(w, sr)
	w = binary.LittleEndian.AppendUint32(w, sr*2)
	w = binary.LittleEndian.AppendUint16(w, 2)
	w = binary.LittleEndian.AppendUint16(w, 16)
	w = append(w, "loop"...)
	w = binary.LittleEndian.AppendUint32(w, 4)
	w = binary.LittleEndian.AppendUint32(w, loopVal)
	w = append(w, "data"...)
	w = binary.LittleEndian.AppendUint32(w, uint32(nb))
	w = append(w, pcm16...)
	return w, fmt.Sprintf("%.1f s", float64(len(buf))/sr), nil
}
