package main

// TASKS.BIN: the 61 test files (ported from tools/export_tasks.py, which documents the layout).

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

var taskKinds = []string{"THER", "LWIN", "TPAD", "LPAD", "LSTP", "RNGS", "BALS", "TARG", "HPAD", "BTGT", "PHTS", "FALC", "SDFM", "CNTG", "HOPD", "OBSV"}
var taskSizes = map[string]int{"THER": 0x28, "LWIN": 0x54, "TPAD": 0x30, "LPAD": 0x30, "LSTP": 0x24, "RNGS": 0x84, "BALS": 0x68, "TARG": 0x20,
	"HPAD": 0x40, "BTGT": 0x1C, "PHTS": 0x14, "FALC": 0xAC, "SDFM": 0x4C, "CNTG": 0x1C, "HOPD": 0x20, "OBSV": 0x10}
var taskFmt = map[string]string{
	"THER": "3f f f i f f f f",
	"LWIN": "15f i 3f f B B 2x",
	"TPAD": "3f 3f 4x 3f B 3x f",
	"LPAD": "3f 3f 4x 3f 4x B 3x",
	"LSTP": "3f 3f 4x B 3x f",
	"RNGS": "3f 3f i 1x B 2x 5i B 3x 5i B B 2x f B B 2x f f B 3x f f f B B B B 16x",
	"BALS": "3f 12x i f B B 2x f f f f i i f f B 3x f f f i i f f f",
	"TARG": "3f 3f B B 2x i",
	"HPAD": "3f 3f i B B 2x f B 3x 5i B 3x",
	"BTGT": "3f i f f B B 2x",
	"PHTS": "i i f f f",
	"CNTG": "3f 3f B 3x",
	"HOPD": "i 3f i f f 4x",
	"OBSV": "4f",
	"FALC": "3f f B B B B 4x 37f",
	"SDFM": "",
}

type fmtTok struct {
	n    int
	kind byte
}

func parseFmt(f string) []fmtTok {
	var out []fmtTok
	for _, tok := range strings.Fields(f) {
		n := 0
		i := 0
		for i < len(tok) && tok[i] >= '0' && tok[i] <= '9' {
			n = n*10 + int(tok[i]-'0')
			i++
		}
		if i == 0 {
			n = 1
		}
		out = append(out, fmtTok{n, tok[i]})
	}
	return out
}

func swap(raw []byte, f string) []byte {
	var out []byte
	o := 0
	for _, t := range parseFmt(f) {
		switch t.kind {
		case 'f', 'i':
			for k := 0; k < t.n; k++ {
				out = append(out, raw[o+3], raw[o+2], raw[o+1], raw[o])
				o += 4
			}
		default:
			out = append(out, raw[o:o+t.n]...)
			o += t.n
		}
	}
	return out
}

func checkFormats() error {
	for k, f := range taskFmt {
		if k == "SDFM" {
			continue
		}
		t := 0
		for _, tk := range parseFmt(f) {
			if tk.kind == 'f' || tk.kind == 'i' {
				t += 4 * tk.n
			} else {
				t += tk.n
			}
		}
		if t != taskSizes[k] {
			return fmt.Errorf("task format %s is %d bytes, expected %d", k, t, taskSizes[k])
		}
	}
	return nil
}

func curve(b []byte, o int) []byte {
	var out []byte
	out = append(out, b[o+3], b[o+2], b[o+1], b[o])
	return append(out, swap(b[o+4:o+68], "16f")...)
}

func padTo(b []byte, n int) []byte {
	b = append([]byte{}, b...)
	if len(b) > n {
		b = b[:n]
	}
	for len(b) < n {
		b = append(b, 0)
	}
	return b
}

func buildTask(f form) ([]byte, [3]int, error) {
	ch := map[string][]byte{}
	for _, c := range f.chunks {
		if _, ok := ch[c.tag]; !ok {
			ch[c.tag] = c.data
		}
	}
	c := ch["COMM"]
	if len(c) < 0x42C {
		return nil, [3]int{}, fmt.Errorf("a test file has no COMM chunk")
	}
	counts := c[0x41C:0x42C]
	h := []byte{c[0], c[1], c[2], c[3], c[8], c[4], 0, 0}
	h = append(h, counts...)
	h = append(h, padTo(ch["NAME"], 12)...)
	h = append(h, padTo(ch["JPTX"], 8)...)
	h = append(h, swap(c[0x10:0x2C], "7f")...)
	h = append(h, swap(c[0x2C:0x44], "6f")...)
	h = append(h, swap(c[0x44:0x48], "f")...)
	base := 0x48
	for _, o := range []int{0, 0x54, 0xA8, 0xFC, 0x150} {
		h = append(h, curve(c, base+o)...)
	}
	for k := 0; k < 6; k++ {
		h = append(h, curve(c, base+0x1A4+0x54*k)...)
	}
	for k := 0; k < 4; k++ {
		h = append(h, c[base+0x39C+2*k+1], c[base+0x39C+2*k])
	}
	h = append(h, swap(c[base+0x3A4:base+0x3B4], "4f")...)
	h = append(h, swap(c[base+0x3B4:base+0x3BC], "2i")...)
	h = append(h, swap(c[base+0x3BC:base+0x3CC], "4f")...)
	pos := len(h) + 4*16
	var arrays []byte
	var offs [16]int
	for i, k := range taskKinds {
		n := int(counts[i])
		raw := ch[k]
		var data []byte
		for j := 0; j < n; j++ {
			if (j+1)*taskSizes[k] > len(raw) {
				return nil, [3]int{}, fmt.Errorf("a %s record is cut short", k)
			}
			rec := raw[j*taskSizes[k] : (j+1)*taskSizes[k]]
			if taskFmt[k] != "" {
				data = append(data, swap(rec, taskFmt[k])...)
			} else {
				data = append(data, rec...)
			}
		}
		if n > 0 {
			offs[i] = pos
		}
		arrays = append(arrays, data...)
		pos += len(data)
	}
	for _, o := range offs {
		h = binary.LittleEndian.AppendUint32(h, uint32(o))
	}
	return append(h, arrays...), [3]int{int(c[1]), int(c[0]), int(c[2])}, nil
}

func (r *rom) exportTasks() ([]byte, string, error) {
	if err := checkFormats(); err != nil {
		return nil, "", err
	}
	type task struct {
		key  [3]int
		blob []byte
	}
	var tasks []task
	for _, f := range r.ofType("UPWT") {
		b, k, err := buildTask(f)
		if err != nil {
			return nil, "", err
		}
		tasks = append(tasks, task{k, b})
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i].key, tasks[j].key
		for k := 0; k < 3; k++ {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
	n := len(tasks)
	out := []byte("PWTK")
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, uint16(n))
	off := 8 + 4*n
	var body []byte
	for _, t := range tasks {
		b := t.blob
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		out = binary.LittleEndian.AppendUint32(out, uint32(off+len(body)))
		body = append(body, b...)
	}
	return append(out, body...), fmt.Sprintf("%d tests", n), nil
}
