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
    if len(os.Args) < 2 { fmt.Fprintln(os.Stderr, "usage: envelope_props <file>"); os.Exit(2) }
    buf, err := ioutil.ReadFile(os.Args[1])
    if err != nil { panic(err) }
    prefix := []byte{0x55, 0x4B, 0xBB, 0xEC}
    actorFreq := map[string]int{}
    propFreq := map[string]int{}
    actorPropFreq := map[string]int{}
    for i := 0; i+20 <= len(buf); i++ {
        if buf[i] != prefix[0] || buf[i+1] != prefix[1] || buf[i+2] != prefix[2] || buf[i+3] != prefix[3] { continue }
        actor := hex.EncodeToString(buf[i+4 : i+12])
        prop := hex.EncodeToString(buf[i+12 : i+20])
        actorFreq[actor]++
        propFreq[prop]++
        actorPropFreq[actor+":"+prop]++
    }
    fmt.Println("actors:")
    actorPairs := make([]pair, 0, len(actorFreq))
    for k,n := range actorFreq { if n >= 5 { actorPairs = append(actorPairs, pair{k,n}) } }
    sort.Slice(actorPairs, func(i,j int) bool { return actorPairs[i].n > actorPairs[j].n })
    for i,p := range actorPairs { if i==50 { break }; fmt.Printf("%s %d\n", p.key, p.n) }
    fmt.Println("props:")
    propPairs := make([]pair, 0, len(propFreq))
    for k,n := range propFreq { if n >= 5 { propPairs = append(propPairs, pair{k,n}) } }
    sort.Slice(propPairs, func(i,j int) bool { return propPairs[i].n > propPairs[j].n })
    for i,p := range propPairs { if i==50 { break }; fmt.Printf("%s %d\n", p.key, p.n) }
    fmt.Println("actorProps:")
    apPairs := make([]pair, 0, len(actorPropFreq))
    for k,n := range actorPropFreq { if n >= 5 { apPairs = append(apPairs, pair{k,n}) } }
    sort.Slice(apPairs, func(i,j int) bool { return apPairs[i].n > apPairs[j].n })
    for i,p := range apPairs { if i==100 { break }; fmt.Printf("%s %d\n", p.key, p.n) }
}
