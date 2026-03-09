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
    tails map[string]int
    min [4]float32
    max [4]float32
    samples []string
}

func main() {
    if len(os.Args) < 3 {
        fmt.Fprintln(os.Stderr, "usage: actor_records <file> <actor-id-hex>")
        os.Exit(2)
    }
    buf, err := ioutil.ReadFile(os.Args[1])
    if err != nil { panic(err) }
    actor, err := hex.DecodeString(os.Args[2])
    if err != nil { panic(err) }
    recs := map[string]*stats{}
    for i := 0; i+40 <= len(buf); i++ {
        ok := true
        for j := range actor {
            if buf[i+j] != actor[j] { ok = false; break }
        }
        if !ok { continue }
        flags := hex.EncodeToString(buf[i+8 : i+12])
        prop := hex.EncodeToString(buf[i+12 : i+20])
        tail := hex.EncodeToString(buf[i+36 : i+40])
        var f [4]float32
        for k := 0; k < 4; k++ {
            bits := binary.LittleEndian.Uint32(buf[i+20+k*4 : i+24+k*4])
            f[k] = math.Float32frombits(bits)
        }
        s := recs[prop]
        if s == nil {
            s = &stats{flags: map[string]int{}, tails: map[string]int{}}
            for k := range s.min { s.min[k] = float32(math.MaxFloat32); s.max[k] = -float32(math.MaxFloat32) }
            recs[prop] = s
        }
        s.count++
        s.flags[flags]++
        s.tails[tail]++
        for k := range f {
            if f[k] < s.min[k] { s.min[k] = f[k] }
            if f[k] > s.max[k] { s.max[k] = f[k] }
        }
        if len(s.samples) < 3 {
            s.samples = append(s.samples, fmt.Sprintf("flags=%s floats=[%.4f %.4f %.4f %.4f] tail=%s", flags, f[0], f[1], f[2], f[3], tail))
        }
    }
    type pair struct { prop string; s *stats }
    pairs := make([]pair, 0, len(recs))
    for prop, s := range recs { if s.count >= 5 { pairs = append(pairs, pair{prop, s}) } }
    sort.Slice(pairs, func(i,j int) bool { return pairs[i].s.count > pairs[j].s.count })
    for _, p := range pairs {
        fmt.Printf("prop=%s count=%d min=[%.4f %.4f %.4f %.4f] max=[%.4f %.4f %.4f %.4f]\n", p.prop, p.s.count, p.s.min[0], p.s.min[1], p.s.min[2], p.s.min[3], p.s.max[0], p.s.max[1], p.s.max[2], p.s.max[3])
        fmt.Printf("  sample %s\n", p.s.samples[0])
        if len(p.s.samples) > 1 { fmt.Printf("  sample %s\n", p.s.samples[1]) }
        if len(p.s.samples) > 2 { fmt.Printf("  sample %s\n", p.s.samples[2]) }
    }
}
