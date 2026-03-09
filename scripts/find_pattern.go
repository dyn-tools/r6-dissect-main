package main

import (
    "encoding/hex"
    "fmt"
    "io/ioutil"
    "os"
    "sort"
)

type hit struct {
    offset int
    before []byte
    after  []byte
}

func findAll(buf, needle []byte, limit int) []hit {
    hits := make([]hit, 0)
    for i := 0; i <= len(buf)-len(needle); i++ {
        ok := true
        for j := range needle {
            if buf[i+j] != needle[j] {
                ok = false
                break
            }
        }
        if !ok {
            continue
        }
        start := i - 16
        if start < 0 {
            start = 0
        }
        end := i + len(needle) + 32
        if end > len(buf) {
            end = len(buf)
        }
        h := hit{offset: i, before: append([]byte{}, buf[start:i]...), after: append([]byte{}, buf[i+len(needle):end]...)}
        hits = append(hits, h)
        if limit > 0 && len(hits) >= limit {
            return hits
        }
    }
    return hits
}

func topPrefixes(buf, needle []byte, prefixLen int) map[string]int {
    freq := make(map[string]int)
    for i := prefixLen; i <= len(buf)-len(needle); i++ {
        ok := true
        for j := range needle {
            if buf[i+j] != needle[j] {
                ok = false
                break
            }
        }
        if !ok {
            continue
        }
        key := hex.EncodeToString(buf[i-prefixLen : i])
        freq[key]++
    }
    return freq
}

func main() {
    if len(os.Args) < 3 {
        fmt.Fprintln(os.Stderr, "usage: find_pattern <file> <hex> [prefixLen]")
        os.Exit(2)
    }
    buf, err := ioutil.ReadFile(os.Args[1])
    if err != nil {
        panic(err)
    }
    needle, err := hex.DecodeString(os.Args[2])
    if err != nil {
        panic(err)
    }
    fmt.Printf("needle=%s len=%d\n", os.Args[2], len(needle))
    hits := findAll(buf, needle, 20)
    fmt.Printf("sample_hits=%d\n", len(hits))
    for _, h := range hits {
        fmt.Printf("offset=%d before=%s match=%s after=%s\n", h.offset, hex.EncodeToString(h.before), os.Args[2], hex.EncodeToString(h.after))
    }
    if len(os.Args) >= 4 {
        var prefixLen int
        _, err = fmt.Sscanf(os.Args[3], "%d", &prefixLen)
        if err != nil {
            panic(err)
        }
        freq := topPrefixes(buf, needle, prefixLen)
        type pair struct {
            k string
            v int
        }
        pairs := make([]pair, 0, len(freq))
        for k, v := range freq {
            pairs = append(pairs, pair{k: k, v: v})
        }
        sort.Slice(pairs, func(i, j int) bool {
            if pairs[i].v == pairs[j].v {
                return pairs[i].k < pairs[j].k
            }
            return pairs[i].v > pairs[j].v
        })
        fmt.Printf("top_prefixes(%d):\n", prefixLen)
        for i, p := range pairs {
            if i == 20 {
                break
            }
            fmt.Printf("%s %d\n", p.k, p.v)
        }
    }
}
