package main

// The sound bank (the UVSX file's .CTL and .TBL: instruments, envelopes, VADPCM books and samples) shared by the effects and the music.

import (
	"encoding/binary"
	"fmt"
	"math"
)

type bank struct {
	ctl, tbl []byte
	b0       int
	nInst    int
	rate     int
}

func newBank(r *rom) (*bank, error) {
	fs := r.ofType("UVSX")
	if len(fs) == 0 || len(fs[0].chunks) < 2 {
		return nil, fmt.Errorf("the ROM has no sound bank")
	}
	b := &bank{ctl: fs[0].chunks[0].data, tbl: fs[0].chunks[1].data}
	b.b0 = b.u32(4)
	b.nInst = s16(b.ctl[b.b0:])
	b.rate = int(int32(binary.BigEndian.Uint32(b.ctl[b.b0+4:])))
	return b, nil
}
func (b *bank) u32(o int) int { return int(binary.BigEndian.Uint32(b.ctl[o:])) }
func (b *bank) s32(o int) int { return int(int32(binary.BigEndian.Uint32(b.ctl[o:]))) }

// vadpcm decodes N64 VADPCM (9-byte frames of 16 samples) to 16-bit values.
func vadpcm(data []byte, book []int, order, npred int) []float64 {
	nfr := len(data) / 9
	out := make([]float64, nfr*16)
	h1, h2 := 0, 0
	at := func(p, r, j int) int { return book[(p*order+r)*8+j] }
	for f := 0; f < nfr; f++ {
		fr := data[f*9 : f*9+9]
		scale := uint(fr[0] >> 4)
		pred := int(fr[0] & 15)
		if pred >= npred {
			pred = 0
		}
		var nib [16]int
		for i, by := range fr[1:] {
			for k, n := range [2]int{int(by>>4) & 15, int(by) & 15} {
				if n > 7 {
					n -= 16
				}
				nib[i*2+k] = n << scale
			}
		}
		for half := 0; half < 2; half++ {
			in := nib[half*8 : half*8+8]
			var res [8]int
			for j := 0; j < 8; j++ {
				acc := in[j] << 11
				if order >= 2 {
					acc += at(pred, 0, j)*h2 + at(pred, 1, j)*h1
				} else {
					acc += at(pred, 0, j) * h1
				}
				for k := 0; k < j; k++ {
					if order >= 2 {
						acc += in[k] * at(pred, 1, j-1-k)
					} else {
						acc += in[k] * at(pred, 0, j-1-k)
					}
				}
				v := acc >> 11
				if v > 32767 {
					v = 32767
				} else if v < -32768 {
					v = -32768
				}
				res[j] = v
				out[f*16+half*8+j] = float64(v)
			}
			h2, h1 = res[6], res[7]
		}
	}
	return out
}

type sound struct {
	pcm                       []float64 // 16-bit units
	loop                      bool
	ls, le                    int
	atk, dec, rel, av, dv     float64
	kbase, det, kmin, kmax    int
	vmin, vmax                int
	svol, vol                 float64
	rawLoopS, rawLoopE, rawLC int
}

// sample reads one sound of an instrument (so: the offset of its sound record).
func (b *bank) sample(so int, withEnv bool) sound {
	wt := b.u32(so + 8)
	base, ln := b.u32(wt), b.s32(wt+4)
	loop, bk := b.u32(wt+12), b.u32(wt+16)
	order, npred := b.s32(bk), b.s32(bk+4)
	book := make([]int, order*npred*8)
	for i := range book {
		book[i] = s16(b.ctl[bk+8+2*i:])
	}
	end := base + (ln+8)/9*9
	if end > len(b.tbl) {
		end = len(b.tbl)
	}
	pcm := vadpcm(b.tbl[base:end], book, order, npred)
	var s sound
	s.pcm = pcm
	if loop != 0 {
		a, e, c := b.u32(loop), b.u32(loop+4), b.u32(loop+8)
		s.rawLoopS, s.rawLoopE, s.rawLC = a, e, c
	}
	return s
}

func (b *bank) instrument(prog int) []sound {
	if prog >= b.nInst {
		return nil
	}
	ins := b.u32(b.b0 + 12 + 4*prog)
	if ins == 0 {
		return nil
	}
	vol := float64(b.ctl[ins])
	nsnd := s16(b.ctl[ins+14:])
	var out []sound
	for i := 0; i < nsnd; i++ {
		so := b.u32(ins + 16 + 4*i)
		env, km := b.u32(so), b.u32(so+4)
		s := b.sample(so, true)
		s.atk, s.dec, s.rel = float64(b.s32(env))/1e6, float64(b.s32(env+4))/1e6, float64(b.s32(env+8))/1e6
		s.av, s.dv = float64(b.ctl[env+12])/127.0, float64(b.ctl[env+13])/127.0
		s.vmin, s.vmax, s.kmin, s.kmax, s.kbase = int(b.ctl[km]), int(b.ctl[km+1]), int(b.ctl[km+2]), int(b.ctl[km+3]), int(b.ctl[km+4])
		s.det = int(int8(b.ctl[km+5]))
		if s.rawLC != 0 {
			s.loop, s.ls, s.le = true, s.rawLoopS, int(math.Min(float64(s.rawLoopE), float64(len(s.pcm))))
		}
		s.svol = float64(b.ctl[so+13]) / 127.0
		s.vol = vol / 127.0
		for k := range s.pcm {
			s.pcm[k] /= 32768.0
		}
		out = append(out, s)
	}
	return out
}
