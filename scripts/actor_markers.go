package main

import (
    "encoding/hex"
    "fmt"
    "io/ioutil"
    "os"
    "sort"
)

type pair struct { key string; n int }

func main() {
    if len(os.Args) < 3 {
        fmt.Fprintln(os.Stderr, "usage: actor_markers <file> <id-hex>")
        os.Exit(2)
    }
    buf, err := ioutil.ReadFile(os.Args[1])
    if err != nil { panic(err) }
    needle, err := hex.DecodeString(os.Args[2])
    if err != nil { panic(err) }
    freq := map[string]int{}
    for i := 0; i <= len(buf)-len(needle); i++ {
        ok := true
        for j := range needle {
            if buf[i+j] != needle[j] { ok = false; break }
        }
        if !ok { continue }
        end := i + len(needle) + 24
        if end > len(buf) { end = len(buf) }
        for j := i + len(needle); j+5 <= end; j++ {
            if buf[j] == 0x22 || buf[j] == 0x23 {
                key := hex.EncodeToString(buf[j : j+5])
                freq[key]++
            }
        }
    }
    pairs := make([]pair, 0, len(freq))
    for k, n := range freq { pairs = append(pairs, pair{k,n}) }
    sort.Slice(pairs, func(i,j int) bool { if pairs[i].n == pairs[j].n { return pairs[i].key < pairs[j].key }; return pairs[i].n > pairs[j].n })
    for i,p := range pairs {
        if i == 50 { break }
        fmt.Printf("%s %d\n", p.key, p.n)
    }
}
