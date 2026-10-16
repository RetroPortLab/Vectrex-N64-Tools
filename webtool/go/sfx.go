package main

// SFX.BIN: the sound effects the game uses, 16-bit mono at 32 kHz (ported from tools/export_sfx.py, which documents the file).

import (
	"encoding/binary"
	"fmt"
	"math"
)

const sr = 32000

var sfxIDs = []int{0x00, 0x01, 0x04, 0x05, 0x0C, 0x0F, 0x11, 0x13, 0x16, 0x17, 0x18, 0x1A, 0x1E, 0x20, 0x22, 0x23, 0x24, 0x37, 0x38, 0x6C, 0x6D, 0x31, 0x35, 0x36, 0x39, 0x3D, 0x55, 0x5C, 0x6A, 0x6E, 0x1F, 0x42, 0x2F, 0x30, 0x3A}
var sfxTrim = map[int]float64{0x11: 0.8, 0x55: 0.7, 0x04: 0.5, 0x39: 0.4, 0x2F: 0.4, 0x30: 0.5, 0x3A: 0.5}

func (r *rom) exportSFX() ([]byte, string, error) {
	bk, err := newBank(r)
	if err != nil {
		return nil, "", err
	}
	type ent struct{ id, looped, start, n, ls, le int }
	var ents []ent
	var pcmAll []int16
	pos := 0
	for _, sid := range sfxIDs {
		ins := 0
		if sid < bk.nInst {
			ins = bk.u32(bk.b0 + 12 + 4*sid)
		}
		if ins == 0 {
			continue
		}
		so := bk.u32(ins + 16)
		s := bk.sample(so, false)
		wt := bk.u32(so + 8)
		ln := bk.s32(wt + 4)
		pcm := s.pcm
		if ln < len(pcm) {
			pcm = pcm[:ln]
		}
		ls, le, looped := 0, 0, 0
		if s.rawLC != 0 {
			ls, le, looped = s.rawLoopS, min(s.rawLoopE, len(pcm)), 1
		}
		if sid == 0x2F || sid == 0x30 {
			looped = 0
		}
		rr := float64(bk.rate) / float64(sr)
		n := int(float64(len(pcm)-1) / rr)
		x := make([]float64, n)
		for k := 0; k < n; k++ {
			p := float64(k) * rr
			j := int(math.Floor(p))
			if j >= len(pcm)-1 {
				x[k] = pcm[len(pcm)-1]
				continue
			}
			slope := pcm[j+1] - pcm[j]
			x[k] = slope*(p-float64(j)) + pcm[j]
		}
		if looped != 0 {
			ls, le = int(float64(ls)/rr), min(n, int(float64(le)/rr))
		}
		if t, ok := sfxTrim[sid]; ok && looped == 0 && n > int(t*sr) {
			n = int(t * sr)
			x = x[:n]
			fd := int(0.04 * sr)
			step := (0.0 - 1.0) / float64(fd-1)
			for i := 0; i < fd; i++ {
				w := float64(i)*step + 1.0
				if i == fd-1 {
					w = 0.0
				}
				x[n-fd+i] *= w
			}
		}
		ents = append(ents, ent{sid, looped, pos, n, ls, le})
		for _, v := range x {
			v = math.Max(-32768, math.Min(32767, v))
			pcmAll = append(pcmAll, int16(int(v)))
		}
		pos += n
	}
	out := []byte("PWSX")
	out = binary.LittleEndian.AppendUint32(out, uint32(len(ents)))
	for _, e := range ents {
		out = binary.LittleEndian.AppendUint16(out, uint16(e.id))
		out = binary.LittleEndian.AppendUint16(out, uint16(e.looped))
		for _, v := range []int{e.start, e.n, e.ls, e.le} {
			out = binary.LittleEndian.AppendUint32(out, uint32(v))
		}
	}
	for _, v := range pcmAll {
		out = binary.LittleEndian.AppendUint16(out, uint16(v))
	}
	return out, fmt.Sprintf("%d sound effects", len(ents)), nil
}
