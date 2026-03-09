package main

import (
    "encoding/hex"
    "fmt"
    "io/ioutil"
    "os"
    "sort"
)

type pair struct {
    key string
    n   int
}

func main() {
    if len(os.Args) < 4 {
        fmt.Fprintln(os.Stderr, "usage: freq_windows <file> <window> <firstbyte-hex>")
        os.Exit(2)
    }
    buf, err := ioutil.ReadFile(os.Args[1])
    if err != nil {
        panic(err)
    }
    var window int
    if _, err := fmt.Sscanf(os.Args[2], "%d", &window); err != nil {
        panic(err)
    }
    var first byte
    if _, err := fmt.Sscanf(os.Args[3], "%x", &first); err != nil {
        panic(err)
    }
    freq := make(map[string]int)
    for i := 0; i <= len(buf)-window; i++ {
        if buf[i] != first {
            continue
        }
        key := hex.EncodeToString(buf[i : i+window])
        freq[key]++
    }
    pairs := make([]pair, 0, len(freq))
    for k, n := range freq {
        if n < 10 {
            continue
        }
        pairs = append(pairs, pair{key: k, n: n})
    }
    sort.Slice(pairs, func(i, j int) bool {
        if pairs[i].n == pairs[j].n {
            return pairs[i].key < pairs[j].key
        }
        return pairs[i].n > pairs[j].n
    })
    for i, p := range pairs {
        if i == 100 {
            break
        }
        fmt.Printf("%s %d\n", p.key, p.n)
    }
}
