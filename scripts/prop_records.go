package main

import (
    "encoding/binary"
    "encoding/hex"
    "fmt"
    "io/ioutil"
    "math"
    "os"
    "sort"
)

type stats struct {
    count int
    flags map[string]int
    min [4]float32
    max [4]float32
    samples []string
}

func main() {
    if len(os.Args) < 3 {
        fmt.Fprintln(os.Stderr, "usage: prop_records <file> <prop-hex>")
        os.Exit(2)
    }
    buf, err := ioutil.ReadFile(os.Args[1])
    if err != nil { panic(err) }
    prop, err := hex.DecodeString(os.Args[2])
    if err != nil { panic(err) }
    recs := map[string]*stats{}
    for i := 16; i+len(prop)+20 <= len(buf); i++ {
        ok := true
        for j := range prop {
            if buf[i+j] != prop[j] { ok = false; break }
        }
        if !ok { continue }
        actor := hex.EncodeToString(buf[i-12 : i-4])
        flags := hex.EncodeToString(buf[i-4 : i])
        var f [4]float32
        for k := 0; k < 4; k++ {
            bits := binary.LittleEndian.Uint32(buf[i+len(prop)+k*4 : i+len(prop)+k*4+4])
            f[k] = math.Float32frombits(bits)
        }
        s := recs[actor]
        if s == nil {
            s = &stats{flags: map[string]int{}}
            for k := range s.min { s.min[k] = float32(math.MaxFloat32); s.max[k] = -float32(math.MaxFloat32) }
            recs[actor] = s
        }
        s.count++
        s.flags[flags]++
        for k := range f {
            if f[k] < s.min[k] { s.min[k] = f[k] }
            if f[k] > s.max[k] { s.max[k] = f[k] }
        }
        if len(s.samples) < 3 {
            s.samples = append(s.samples, fmt.Sprintf("flags=%s floats=[%.4f %.4f %.4f %.4f]", flags, f[0], f[1], f[2], f[3]))
        }
    }
    type pair struct { actor string; s *stats }
    pairs := make([]pair, 0, len(recs))
    for actor, s := range recs { if s.count >= 3 { pairs = append(pairs, pair{actor, s}) } }
    sort.Slice(pairs, func(i,j int) bool { return pairs[i].s.count > pairs[j].s.count })
    for _, p := range pairs {
        fmt.Printf("actor=%s count=%d min=[%.4f %.4f %.4f %.4f] max=[%.4f %.4f %.4f %.4f]\n", p.actor, p.s.count, p.s.min[0], p.s.min[1], p.s.min[2], p.s.min[3], p.s.max[0], p.s.max[1], p.s.max[2], p.s.max[3])
        for _, sample := range p.s.samples { fmt.Printf("  sample %s\n", sample) }
    }
}
