package main

// TEXT.BIN: the tests' own words (ported from tools/pwtext.py and tools/export_text.py). The cartridge reads it into PSRAM; the program itself carries none of the ROM's text.
//
//   header   8 bytes "PWTX", u16 version, u16 0; then the badge table, 4 classes x 4 (bronze, silver, gold, perfect) u16: 32 bytes
//   entries  100 x 36 bytes: 36 hang glider / rocket belt / gyrocopter tests [craft][class][test], then 64 extra-craft tests [craft][level][test]
//            u32 name, mission, hint, row[4] (offsets from the start of the file, 0 = none), u8 nrows, u8 max[4], 3 bytes padding
//   strings  NUL terminated, upper case (the Vectrex font has no lower case), lines separated by \n

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var punct = map[int]string{73: ".", 74: "", 67: "!", 68: "(", 69: ")", 70: "-", 71: ":", 72: ",", 75: "!", 76: "?", 158: "-", 167: "&", 62: "*", 171: "*",
	77: "'", 78: "?", 79: "!", 80: ",", 81: ".", 82: "-", 83: ":", 84: "/", 85: "(", 86: ")", 87: "\""}

func (r *rom) texts() (map[string]string, error) {
	fs := r.ofType("ADAT")
	if len(fs) == 0 {
		return nil, fmt.Errorf("the ROM has no text file")
	}
	var names []string
	var datas [][]byte
	for _, c := range fs[0].chunks {
		if c.tag == "NAME" {
			n := c.data
			for i, b := range n {
				if b == 0 {
					n = n[:i]
					break
				}
			}
			names = append(names, string(n))
		} else if c.tag == "DATA" {
			datas = append(datas, c.data)
		}
	}
	out := map[string]string{}
	for k := 0; k < len(names) && k < len(datas); k++ {
		d := datas[k]
		var sb strings.Builder
		for i := 0; i+2 <= len(d); {
			c := be16(d[i:])
			i += 2
			switch {
			case c == 253:
				i += 4
			case c == 255:
				i = len(d)
			case c == 254:
				sb.WriteByte('\n')
			case c == 66:
				sb.WriteByte(' ')
			case c >= 106 && c <= 131:
				sb.WriteByte(byte(c - 41))
			case c >= 132 && c <= 157:
				sb.WriteByte(byte(c - 35))
			case c >= 96 && c <= 105:
				sb.WriteByte(byte(c - 48))
			case c <= 9:
				sb.WriteByte(byte(c + 48))
			case c >= 10 && c <= 35:
				sb.WriteByte(byte(c + 55))
			case c >= 36 && c <= 61:
				sb.WriteByte(byte(c + 61))
			default:
				if p, ok := punct[c]; ok {
					sb.WriteString(p)
				} else if c == 160 || c == 161 {
				} else {
					sb.WriteString("{" + strconv.Itoa(c) + "}")
				}
			}
		}
		out[names[k]] = sb.String()
	}
	return out, nil
}

var reBrace = regexp.MustCompile(`\{\d+\}`)
var reRow = regexp.MustCompile(`^(.*?)\s+(\d+)\s*pts`)
var reNum = regexp.MustCompile(`\d+`)

type trow struct {
	label string
	pts   int
}

func cleanText(s string) string { return strings.Trim(reBrace.ReplaceAllString(s, ""), "\n ") }
func rowsOf(txt string) []trow {
	var rows []trow
	for _, ln := range strings.Split(txt, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Total") {
			continue
		}
		if m := reRow.FindStringSubmatch(ln); m != nil {
			n, _ := strconv.Atoi(m[2])
			rows = append(rows, trow{strings.TrimSpace(m[1]), n})
		}
	}
	return rows
}

type entry struct {
	name, mission, hint string
	rows                []trow
	set                 bool
}

func (r *rom) exportText() ([]byte, string, error) {
	T, err := r.texts()
	if err != nil {
		return nil, "", err
	}
	var missing []string
	get := func(k string) string {
		v, ok := T[k]
		if !ok {
			missing = append(missing, k)
		}
		return v
	}
	cl := []string{"E", "A", "B", "P"}
	nst := []int{1, 2, 3, 3}
	var ents []entry
	for _, veh := range []string{"HG", "RP", "GC"} {
		for c := 0; c < 4; c++ {
			for st := 0; st < 3; st++ {
				if st >= nst[c] {
					ents = append(ents, entry{})
					continue
				}
				L := cl[c]
				k := fmt.Sprintf("%s_%s_%d_", L, veh, st+1)
				sk := fmt.Sprintf("%s_%s%d_S2", veh, L, st+1)
				if c == 0 {
					sk = veh + "_E_S2"
				}
				ents = append(ents, entry{cleanText(get(k + "N")), cleanText(get(k + "M")), cleanText(get(k + "H")), rowsOf(get(sk)), true})
			}
		}
	}
	type xc struct{ nlev, nst int }
	for craft, x := range []xc{{3, 4}, {3, 1}, {3, 1}, {4, 4}} {
		for lv := 0; lv < 4; lv++ {
			for st := 0; st < 4; st++ {
				if lv >= x.nlev || st >= x.nst {
					ents = append(ents, entry{})
					continue
				}
				var k string
				var rows []trow
				if craft == 3 {
					k = fmt.Sprintf("%s_BD_%d_", cl[lv], st+1)
				} else {
					k = fmt.Sprintf("%s_EX_%d_", []string{"A", "B", "P"}[craft], lv+1)
					rows = rowsOf(get([]string{"CB_L123_S1", "SD_L123_S1", fmt.Sprintf("HP_L%d_S1", lv+1)}[craft]))
				}
				ents = append(ents, entry{cleanText(get(k + "N")), cleanText(get(k + "M")), cleanText(get(k + "H")), rows, true})
			}
		}
	}
	var badge [16]int
	for c := 0; c < 4; c++ {
		for i, n := range []string{"BLONDS", "SILVER", "GOLD", "PERFECT"} {
			v := 0
			if m := reNum.FindString(get(fmt.Sprintf("%s_S3_%s", cl[c], n))); m != "" {
				v, _ = strconv.Atoi(m)
			}
			badge[c*4+i] = v
		}
	}
	if len(missing) > 0 {
		return nil, "", fmt.Errorf("the ROM's text lacks %s: is this the USA ROM?", strings.Join(missing[:min(3, len(missing))], ", "))
	}
	const hdr = 8 + 32 + 100*36
	blob := []byte{}
	strs := map[string]int{}
	put := func(s string) uint32 {
		s = strings.ToUpper(s)
		if o, ok := strs[s]; ok {
			return uint32(o)
		}
		o := hdr + len(blob)
		strs[s] = o
		blob = append(append(blob, s...), 0)
		return uint32(o)
	}
	out := []byte("PWTX")
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 0)
	for _, b := range badge {
		out = binary.LittleEndian.AppendUint16(out, uint16(b))
	}
	for _, e := range ents {
		if !e.set {
			out = append(out, make([]byte, 36)...)
			continue
		}
		out = binary.LittleEndian.AppendUint32(out, put(e.name))
		out = binary.LittleEndian.AppendUint32(out, put(e.mission))
		out = binary.LittleEndian.AppendUint32(out, put(e.hint))
		for i := 0; i < 4; i++ {
			if i < len(e.rows) {
				out = binary.LittleEndian.AppendUint32(out, put(e.rows[i].label))
			} else {
				out = binary.LittleEndian.AppendUint32(out, 0)
			}
		}
		out = append(out, byte(len(e.rows)))
		for i := 0; i < 4; i++ {
			if i < len(e.rows) {
				out = append(out, byte(e.rows[i].pts))
			} else {
				out = append(out, 0)
			}
		}
		out = append(out, 0, 0, 0)
	}
	if len(out) != hdr {
		return nil, "", fmt.Errorf("text table size")
	}
	return append(out, blob...), fmt.Sprintf("%d texts", len(strs)), nil
}
