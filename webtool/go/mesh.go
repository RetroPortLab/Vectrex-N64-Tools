package main

// Mesh helpers: welded vertices, triangles and creased edges (ported from tools/pwmesh.py; renderer axes x right, y up, z forward).

import (
	"math"
	"sort"
)

const nearMaxTris = 110

type v3 [3]float64

func sub(a, b v3) v3 { return v3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func cross(a, b v3) v3 {
	return v3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func dot(a, b v3) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func norm(a v3) float64   { return math.Sqrt(dot(a, a)) }

type edge struct {
	a, b, t0, t1, sharp int
	l                   float64
}
type ekey [2][3]int

func vkLess(a, b [3]int) bool {
	for j := 0; j < 3; j++ {
		if a[j] != b[j] {
			return a[j] < b[j]
		}
	}
	return false
}

// buildEdges welds vertices to a grid, collects each welded edge once (first occurrence wins, in order) and marks creases.
func buildEdges(P []v3, tris [][3]int, weld, minlen, creaseDeg float64, skip func(a, b int) bool) ([]edge, []v3) {
	wkey := func(i int) [3]int {
		return [3]int{int(math.Floor(P[i][0]/weld + 0.5)), int(math.Floor(P[i][1]/weld + 0.5)), int(math.Floor(P[i][2]/weld + 0.5))}
	}
	nrm := make([]v3, len(tris))
	for i, t := range tris {
		n := cross(sub(P[t[1]], P[t[0]]), sub(P[t[2]], P[t[0]]))
		l := norm(n)
		if l > 1e-9 {
			nrm[i] = v3{n[0] / l, n[1] / l, n[2] / l}
		}
	}
	type ent struct {
		a, b int
		t    []int
	}
	var ents []*ent
	idx := map[ekey]*ent{}
	for ti, t := range tris {
		for _, uv := range [3][2]int{{t[0], t[1]}, {t[1], t[2]}, {t[2], t[0]}} {
			ka, kb := wkey(uv[0]), wkey(uv[1])
			k := ekey{ka, kb}
			if vkLess(kb, ka) {
				k = ekey{kb, ka}
			}
			if k[0] == k[1] {
				continue
			}
			e := idx[k]
			if e == nil {
				e = &ent{a: uv[0], b: uv[1]}
				idx[k] = e
				ents = append(ents, e)
			}
			if len(e.t) < 2 {
				e.t = append(e.t, ti)
			}
		}
	}
	pi := math.Pi
	cth := math.Cos(creaseDeg * (pi / 180.0))
	var res []edge
	for _, e := range ents {
		L := norm(sub(P[e.a], P[e.b]))
		if L < minlen {
			continue
		}
		if skip != nil && skip(e.a, e.b) {
			continue
		}
		r := edge{a: e.a, b: e.b, t0: e.t[0], t1: -1, l: L}
		if len(e.t) == 1 {
			r.sharp = 1
		} else {
			r.t1 = e.t[1]
			if dot(nrm[e.t[0]], nrm[e.t[1]]) < cth {
				r.sharp = 1
			}
		}
		res = append(res, r)
	}
	return res, nrm
}

// modelLods: the finest level of detail with at most nearMaxTris triangles, and the coarsest.
func modelLods(m *model) (int, int) {
	near := len(m.lods) - 1
	for i, l := range m.lods {
		n := 0
		for _, p := range l.parts {
			n += len(p.tris)
		}
		if n <= nearMaxTris {
			near = i
			break
		}
	}
	return near, len(m.lods) - 1
}

// meshOf: the model's triangles (renderer axes), vertices in first-use order.
func meshOf(m *model, lodi int) ([]v3, [][3]int, float64) {
	vk := map[int]int{}
	var V []v3
	var T [][3]int
	vert := func(i int) int {
		if k, ok := vk[i]; ok {
			return k
		}
		p := m.verts[i]
		vk[i] = len(V)
		V = append(V, v3{float64(p[0]), float64(p[2]), float64(p[1])})
		return vk[i]
	}
	for _, p := range m.lods[lodi].parts {
		for _, t := range p.tris {
			a, b, c := t[0], t[1], t[2]
			if a != b && b != c && a != c {
				va := vert(a)
				vc := vert(c)
				vb := vert(b)
				T = append(T, [3]int{va, vc, vb})
			}
		}
	}
	return V, T, m.lods[lodi].radius
}

func sortedKeys(m map[int]bool) []int {
	var k []int
	for i := range m {
		k = append(k, i)
	}
	sort.Ints(k)
	return k
}
