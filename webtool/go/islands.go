package main

// ISL0..3.BIN: the four islands (ported from tools/export_islands.py, which documents the file layout).

import (
	"encoding/binary"
	"fmt"
	"math"
)

var levelChunk = [4]int{1, 3, 5, 10}
var islandNames = [4]string{"Holiday Island", "Crescent Island", "Little States", "Ever-Frost Island"}

type le struct{ b []byte }

func (w *le) u16(v int) { w.b = binary.LittleEndian.AppendUint16(w.b, uint16(int16(v))) }
func (w *le) u32(v int) { w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v)) }
func (w *le) align() {
	for len(w.b)%4 != 0 {
		w.b = append(w.b, 0)
	}
}

type mrow struct{ vb, tb, eb, nv, nt, ne, rad int }

func clamp16(v int) int {
	if v > 32767 {
		return 32767
	}
	return v
}

func i16(v float64) int { return int(v) }

func (r *rom) exportIsland(idx int) ([]byte, string, error) {
	L, err := r.uvlv(levelChunk[idx])
	if err != nil {
		return nil, "", err
	}
	wind, pads := r.upwl(idx)
	type tileSrc struct {
		pl []placement
		c  collision
		p  place
	}
	var tiles []tileSrc
	used := map[int]bool{}
	cell := 0
	for _, t := range L.terras {
		T := r.uvtr(t)
		if cell == 0 {
			cell = int(T.cx)
		}
		for _, p := range T.places {
			pl := r.uvctFull(p.ci)
			tiles = append(tiles, tileSrc{pl, r.uvct(p.ci), p})
			for _, q := range pl {
				used[q.model] = true
			}
		}
	}
	for _, w := range wind {
		used[w.model] = true
	}
	for range pads {
		used[0xd4] = true
	}
	slots := sortedKeys(used)
	slotOf := map[int]int{}
	for i, m := range slots {
		slotOf[m] = i
	}
	var MV []v3
	var MT [][3]int
	var MN []v3
	var ME []edge
	var rows []mrow
	cache := map[[2]int]mrow{}
	for _, m := range slots {
		md := r.uvmd(m)
		near, far := modelLods(&md)
		for _, l := range [2]int{near, far} {
			k := [2]int{m, l}
			if _, ok := cache[k]; !ok {
				V, Tr, rad := meshOf(&md, l)
				E, N := buildEdges(V, Tr, 1.0, 4.0, 35.0, nil)
				rr := mrow{len(MV), len(MT), len(ME), len(V), len(Tr), len(E), int(math.Min(65535, float64(int(rad))))}
				for _, v := range V {
					MV = append(MV, v3{float64(int(v[0])), float64(int(v[1])), float64(int(v[2]))})
				}
				MT = append(MT, Tr...)
				MN = append(MN, N...)
				ME = append(ME, E...)
				cache[k] = rr
			}
			rows = append(rows, cache[k])
		}
	}
	type trec struct{ cx, cz, ymin, ymax, rad, nv, nt, ne, ninst, first, voff, toff, noff, eoff int }
	var recs []trec
	var TV, TT, TN, TE le
	var inst [][5]int
	maxv, maxt, maxe := 0, 0, 0
	tau := 2 * math.Pi
	for _, t := range tiles {
		var P []v3
		for _, v := range t.c.verts {
			P = append(P, v3{float64(v[0]), float64(v[2]), float64(v[1])})
		}
		for i := range P {
			P[i][1] += t.p.z
		}
		var tris [][3]int
		for _, f := range t.c.faces {
			if f.a != f.b && f.b != f.c && f.a != f.c {
				tris = append(tris, [3]int{f.a, f.c, f.b})
			}
		}
		refSet := map[int]bool{}
		for _, tr := range tris {
			for _, i := range tr {
				refSet[i] = true
			}
		}
		ref := sortedKeys(refSet)
		remap := map[int]int{}
		for n, o := range ref {
			remap[o] = n
		}
		var P2 []v3
		for _, o := range ref {
			P2 = append(P2, P[o])
		}
		P = P2
		for i, tr := range tris {
			tris[i] = [3]int{remap[tr[0]], remap[tr[1]], remap[tr[2]]}
		}
		skip := func(a, b int) bool { return P[a][1] <= 1 && P[b][1] <= 1 }
		E, N := buildEdges(P, tris, 0.5, 10.0, 25.0, skip)
		ymin, ymax, rmax := P[0][1], P[0][1], 0.0
		for _, p := range P {
			ymin, ymax = math.Min(ymin, p[1]), math.Max(ymax, p[1])
			rmax = math.Max(rmax, math.Sqrt(p[0]*p[0]+p[2]*p[2]))
		}
		recs = append(recs, trec{pyRound(t.p.x), pyRound(t.p.y), int(ymin), int(ymax), int(rmax) + 1, len(P), len(tris), len(E), len(t.pl), len(inst),
			len(TV.b), len(TT.b), len(TN.b), len(TE.b)})
		for _, v := range P {
			TV.u16(pyRound(v[0]))
			TV.u16(pyRound(v[1]))
			TV.u16(pyRound(v[2]))
		}
		TV.align()
		for _, tr := range tris {
			TT.u16(tr[0])
			TT.u16(tr[1])
			TT.u16(tr[2])
		}
		TT.align()
		for _, n := range N {
			for k := 0; k < 3; k++ {
				TN.u16(pyRound(n[k] * 32767))
			}
		}
		TN.align()
		for _, e := range E {
			TE.u16(e.a)
			TE.u16(e.b)
			TE.u16(e.t0)
			TE.u16(e.t1)
			TE.u16(e.sharp)
			TE.u16(clamp16(int(e.l)))
		}
		for _, q := range t.pl {
			yaw := -math.Atan2(q.mat[1], q.mat[0])
			inst = append(inst, [5]int{slotOf[q.model], pyRound(q.xyz[0] * 16), pyRound((q.xyz[2] + t.p.z) * 16), pyRound(q.xyz[1] * 16),
				((pyRound(yaw/tau*65536) + 32768) & 0xFFFF) - 32768})
		}
		maxv, maxt, maxe = max(maxv, len(P)), max(maxt, len(tris)), max(maxe, len(E))
	}
	var padb, windb le
	for _, p := range pads {
		padb.u16(int(p.x))
		padb.u16(int(p.z))
		padb.u16(int(p.y))
		padb.u16(pyRound(-p.h / tau * 65536))
	}
	for _, w := range wind {
		windb.u16(int(w.x))
		windb.u16(int(w.z))
		windb.u16(int(w.y))
		windb.u16(slotOf[w.model])
	}
	var mvb, mtb, mnb, meb, modb, instb le
	for _, v := range MV {
		mvb.u16(int(v[0]))
		mvb.u16(int(v[1]))
		mvb.u16(int(v[2]))
	}
	mvb.align()
	for _, t := range MT {
		mtb.u16(t[0])
		mtb.u16(t[1])
		mtb.u16(t[2])
	}
	mtb.align()
	for _, n := range MN {
		for k := 0; k < 3; k++ {
			mnb.u16(pyRound(n[k] * 32767))
		}
	}
	mnb.align()
	for _, e := range ME {
		meb.u16(e.a)
		meb.u16(e.b)
		meb.u16(e.t0)
		meb.u16(e.t1)
		meb.u16(e.sharp)
		meb.u16(clamp16(int(e.l)))
	}
	maxmv, maxmt, maxme := 0, 0, 0
	for _, r := range rows {
		modb.u32(r.vb)
		modb.u32(r.tb)
		modb.u32(r.eb)
		modb.u16(r.nv)
		modb.u16(r.nt)
		modb.u16(r.ne)
		modb.u16(r.rad)
		modb.u32(0)
		maxmv, maxmt, maxme = max(maxmv, r.nv), max(maxmt, r.nt), max(maxme, r.ne)
	}
	for _, i := range inst {
		for k := 0; k < 5; k++ {
			instb.u16(i[k])
		}
		instb.u16(0)
	}
	const HDR, TILE = 80, 36
	o := HDR
	var off [13]int // tiles models mv mt mn me inst pads wind tv tt tn te
	adv := func(k, n int) { off[k] = o; o += n }
	adv(0, len(recs)*TILE)
	adv(1, len(modb.b))
	adv(2, len(mvb.b))
	adv(3, len(mtb.b))
	adv(4, len(mnb.b))
	adv(5, len(meb.b))
	adv(6, len(instb.b))
	adv(7, len(padb.b))
	adv(8, len(windb.b))
	adv(9, len(TV.b))
	adv(10, len(TT.b))
	adv(11, len(TN.b))
	adv(12, len(TE.b))
	var tb le
	for _, r := range recs {
		tb.u16(r.cx)
		tb.u16(r.cz)
		tb.u16(r.ymin)
		tb.u16(r.ymax)
		tb.u16(r.rad)
		tb.u16(r.nv)
		tb.u16(r.nt)
		tb.u16(r.ne)
		tb.u16(r.ninst)
		tb.u16(r.first)
		tb.u32(off[9] + r.voff)
		tb.u32(off[10] + r.toff)
		tb.u32(off[11] + r.noff)
		tb.u32(off[12] + r.eoff)
	}
	if len(tb.b) != len(recs)*TILE {
		return nil, "", fmt.Errorf("tile record size")
	}
	var h le
	h.b = append(h.b, "PWIS"...)
	for _, v := range []int{1, idx, len(recs), len(rows), maxv, maxt, maxe, maxmv, maxmt, maxme, len(pads), len(wind), cell, 0} {
		h.u16(v)
	}
	for k := 0; k < 9; k++ {
		h.u32(off[k])
	}
	for len(h.b) < HDR {
		h.b = append(h.b, 0)
	}
	out := append([]byte{}, h.b...)
	for _, p := range [][]byte{tb.b, modb.b, mvb.b, mtb.b, mnb.b, meb.b, instb.b, padb.b, windb.b, TV.b, TT.b, TN.b, TE.b} {
		out = append(out, p...)
	}
	msg := fmt.Sprintf("%s: %d tiles, %d models, %d objects, %d KB", islandNames[idx], len(recs), len(slots), len(inst), len(out)/1024)
	return out, msg, nil
}
