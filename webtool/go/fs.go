package main

// The ROM's filesystem: a run of FORM files made of chunks, some MIO0-compressed (ported from tools/pwfs.py, which has the format notes).

import (
	"encoding/binary"
	"fmt"
)

const fsBase = 0x0DF5B0

type chunk struct {
	tag  string
	data []byte
}
type form struct {
	typ    string
	off    int
	chunks []chunk
}

func be16(b []byte) int { return int(binary.BigEndian.Uint16(b)) }
func be32(b []byte) int { return int(binary.BigEndian.Uint32(b)) }

func mio0(d []byte) ([]byte, error) {
	if len(d) < 16 || string(d[:4]) != "MIO0" {
		return nil, fmt.Errorf("not MIO0 data")
	}
	n, comp, unc := be32(d[4:]), be32(d[8:]), be32(d[12:])
	out := make([]byte, 0, n)
	maskPos := 16
	var mask uint32
	left := 0
	for len(out) < n {
		if left == 0 {
			if maskPos+4 > len(d) {
				return nil, fmt.Errorf("MIO0 data ends early")
			}
			mask = binary.BigEndian.Uint32(d[maskPos:])
			maskPos += 4
			left = 32
		}
		if mask&0x80000000 != 0 {
			if unc >= len(d) {
				return nil, fmt.Errorf("MIO0 data ends early")
			}
			out = append(out, d[unc])
			unc++
		} else {
			if comp+2 > len(d) {
				return nil, fmt.Errorf("MIO0 data ends early")
			}
			v := be16(d[comp:])
			comp += 2
			ln, off := (v>>12)+3, (v&0xFFF)+1
			if off > len(out) {
				return nil, fmt.Errorf("MIO0 back reference out of range")
			}
			for i := 0; i < ln; i++ {
				out = append(out, out[len(out)-off])
			}
		}
		mask <<= 1
		left--
	}
	return out[:n], nil
}

func parseFS(rom []byte) ([]form, error) {
	if len(rom) <= fsBase {
		return nil, fmt.Errorf("the file is too small to be a Pilotwings 64 ROM")
	}
	fs := rom[fsBase:]
	var files []form
	o := 0
	for o+12 <= len(fs) {
		if string(fs[o:o+4]) != "FORM" {
			break
		}
		ln := be32(fs[o+4:])
		typ := string(fs[o+8 : o+12])
		end := o + 8 + ln
		if end > len(fs) {
			return nil, fmt.Errorf("a file in the ROM runs past its end")
		}
		p := o + 12
		var chunks []chunk
		for p < end {
			tag := string(fs[p : p+4])
			sz := be32(fs[p+4:])
			if p+8+sz > len(fs) {
				return nil, fmt.Errorf("a chunk in the ROM runs past its end")
			}
			if tag == "GZIP" {
				tag2 := string(fs[p+8 : p+12])
				dsz := be32(fs[p+12:])
				data, err := mio0(fs[p+16 : p+8+sz])
				if err != nil {
					return nil, err
				}
				if len(data) != dsz {
					return nil, fmt.Errorf("a compressed chunk has the wrong size")
				}
				chunks = append(chunks, chunk{tag2, data})
			} else if tag != "PAD " {
				chunks = append(chunks, chunk{tag, fs[p+8 : p+8+sz]})
			}
			p += 8 + sz
		}
		files = append(files, form{typ, o, chunks})
		o = end
	}
	return files, nil
}
