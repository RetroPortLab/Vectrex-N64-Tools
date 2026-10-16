package main

// Parsers for the level, terrain, collision, model and object files in the ROM (ported from tools/pwdata.py).

import (
	"encoding/binary"
	"fmt"
	"math"
)

type rom struct{ files []form }

func (r *rom) ofType(t string) []form {
	var o []form
	for _, f := range r.files {
		if f.typ == t {
			o = append(o, f)
		}
	}
	return o
}

func s16(b []byte) int      { return int(int16(binary.BigEndian.Uint16(b))) }
func f32(b []byte) float64  { return float64(math.Float32frombits(binary.BigEndian.Uint32(b))) }
func pyRound(x float64) int { return int(math.RoundToEven(x)) }

type level struct{ terras, models []int }

func (r *rom) uvlv(i int) (level, error) {
	fs := r.ofType("UVLV")
	if len(fs) == 0 || i >= len(fs[0].chunks) {
		return level{}, fmt.Errorf("level %d is missing", i)
	}
	d := fs[0].chunks[i].data
	idx := 0
	var lists [10][]int
	for k := 0; k < 10; k++ {
		if idx+2 > len(d) {
			return level{}, fmt.Errorf("level %d is cut short", i)
		}
		n := be16(d[idx:])
		for j := 0; j < n; j++ {
			lists[k] = append(lists[k], be16(d[idx+2+2*j:]))
		}
		idx += 2 + 2*n
	}
	return level{terras: lists[0], models: lists[3]}, nil
}

type place struct {
	ci      int
	x, y, z float64
}
type terrain struct {
	cx     float64
	places []place
}

func (r *rom) uvtr(i int) terrain {
	d := r.ofType("UVTR")[0].chunks[i].data
	gw, gh := int(d[0x18]), int(d[0x19])
	t := terrain{cx: f32(d[0x1A:])}
	o := 0x26
	for k := 0; k < gw*gh; k++ {
		flag := d[o]
		o++
		if flag == 0 {
			continue
		}
		t.places = append(t.places, place{be16(d[o+0x41:]), f32(d[o+0x30:]), f32(d[o+0x34:]), f32(d[o+0x38:])})
		o += 0x43
	}
	return t
}

type face struct{ a, b, c, fl int }
type collision struct {
	verts [][3]int
	faces []face
}
type placement struct {
	model int
	mat   [16]float64
	hasM  bool
	xyz   [3]float64
}

func (r *rom) uvct(i int) collision {
	d := r.ofType("UVCT")[i].chunks[0].data
	vc, fc := be16(d), be16(d[2:])
	o := 8
	var c collision
	for k := 0; k < vc; k++ {
		c.verts = append(c.verts, [3]int{s16(d[o:]), s16(d[o+2:]), s16(d[o+4:])})
		o += 16
	}
	for k := 0; k < fc; k++ {
		c.faces = append(c.faces, face{be16(d[o:]), be16(d[o+2:]), be16(d[o+4:]), be16(d[o+6:])})
		o += 8
	}
	return c
}

func rdpMtx(d []byte, o int) [16]float64 {
	var m [16]float64
	for i := 0; i < 16; i++ {
		m[i] = (float64(s16(d[o+2*i:]))*65536 + float64(be16(d[o+32+2*i:]))) / 65536.0
	}
	return m
}

func (r *rom) uvctFull(i int) []placement {
	d := r.ofType("UVCT")[i].chunks[0].data
	vc, fc, mc := be16(d), be16(d[2:]), be16(d[4:])
	o := 8 + vc*16 + fc*8
	var pl []placement
	for k := 0; k < mc; k++ {
		cnt := int(d[o])
		o++
		var mats [][16]float64
		for j := 0; j < cnt; j++ {
			mats = append(mats, rdpMtx(d, o))
			o += 64
		}
		mi := s16(d[o:])
		xyz := [3]float64{f32(d[o+2:]), f32(d[o+6:]), f32(d[o+10:])}
		o += 0x12
		if mi >= 0 {
			p := placement{model: mi, xyz: xyz}
			if len(mats) > 0 {
				p.mat, p.hasM = mats[0], true
			}
			pl = append(pl, p)
		}
	}
	return pl
}

type part struct {
	att  int
	tris [][3]int
}
type lod struct {
	parts  []part
	radius float64
}
type model struct {
	verts [][3]int
	lods  []lod
	mats  [][16]float64
}

func (r *rom) uvmd(i int) model {
	d := r.ofType("UVMD")[i].chunks[0].data
	vc := be16(d)
	lodc, trc, unk := int(d[2]), int(d[3]), int(d[4])
	o := 8
	var m model
	for k := 0; k < vc; k++ {
		m.verts = append(m.verts, [3]int{s16(d[o:]), s16(d[o+2:]), s16(d[o+4:])})
		o += 16
	}
	var vb [16]int
	for k := 0; k < lodc; k++ {
		pc := int(d[o])
		o += 2
		var l lod
		for p := 0; p < pc; p++ {
			tc, att := int(d[o]), int(d[o+2])
			o += 3
			pt := part{att: att}
			for t := 0; t < tc; t++ {
				cc := be16(d[o+8:])
				o += 10
				for c := 0; c < cc; c++ {
					ix := be16(d[o:])
					o += 2
					if ix&0x4000 != 0 {
						pt.tris = append(pt.tris, [3]int{vb[(ix>>8)&15], vb[(ix>>4)&15], vb[ix&15]})
					} else {
						lc := int(d[o])
						o++
						w := lc & 15
						for q := 0; q < (lc>>4)+1; q++ {
							vb[(w+q)&15] = (ix & 0x3FFF) + q
						}
					}
				}
			}
			l.parts = append(l.parts, pt)
		}
		l.radius = f32(d[o:])
		o += 4
		m.lods = append(m.lods, l)
	}
	for k := 0; k < trc; k++ {
		var mm [16]float64
		for j := 0; j < 16; j++ {
			mm[j] = f32(d[o+4*j:])
		}
		m.mats = append(m.mats, mm)
		o += 64
	}
	_ = unk
	return m
}

type windObj struct {
	model   int
	x, y, z float64
}
type padObj struct {
	x, y, z, h float64
}

func (r *rom) upwl(i int) ([]windObj, []padObj) {
	f := r.ofType("UPWL")[i]
	var wind []windObj
	var pads []padObj
	for _, c := range f.chunks {
		d := c.data
		switch c.tag {
		case "WOBJ":
			for o := 0; o+16 <= len(d); o += 16 {
				mdl := 0x53
				if d[o+12] == 0 {
					mdl = 0x40
				}
				wind = append(wind, windObj{mdl, f32(d[o:]), f32(d[o+4:]), f32(d[o+8:])})
			}
		case "LPAD":
			for o := 0; o+24 <= len(d); o += 24 {
				pads = append(pads, padObj{f32(d[o:]), f32(d[o+4:]), f32(d[o+8:]), f32(d[o+12:])})
			}
		}
	}
	return wind, pads
}
