package main

import (
    "encoding/binary"
    "fmt"
    "math"
    "os"
)

type Track struct{ Frames int }

func main() {
    data, _ := os.ReadFile(`d:\r6-dissect-main\running_in_arcs_and_circles.dump.bin`)
    f32 := func(off int) float32 { if off < 0 || off+4 > len(data) { return float32(math.NaN()) }; return math.Float32frombits(binary.LittleEndian.Uint32(data[off:off+4])) }
    u32 := func(off int) uint32 { if off < 0 || off+4 > len(data) { return 0 }; return binary.LittleEndian.Uint32(data[off:off+4]) }
    isPlausibleCoord := func(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -2000 && v <= 2000 }
    isPlausibleHeight := func(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -100 && v <= 150 }
    isUnitQ := func(qx, qy, qz, qw float32) bool {
        for _, v := range []float32{qx, qy, qz, qw} {
            if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < -1.01 || v > 1.01 { return false }
        }
        sum := float64(qx*qx) + float64(qy*qy) + float64(qz*qz) + float64(qw*qw)
        return math.Abs(sum-1.0) < 0.05
    }
    tracks := map[uint32]*Track{}
    addFrame := func(entityIDOff, xOff, quatOff int) {
        entityID := u32(entityIDOff)
        if entityID == 0 { return }
        x, y, z := f32(xOff), f32(xOff+4), f32(xOff+8)
        if !isPlausibleCoord(x) || !isPlausibleCoord(y) || !isPlausibleHeight(z) { return }
        if math.Abs(float64(x)) < 0.01 && math.Abs(float64(y)) < 0.01 && math.Abs(float64(z)) < 0.01 { return }
        qx, qy, qz, qw := f32(quatOff), f32(quatOff+4), f32(quatOff+8), f32(quatOff+12)
        if !isUnitQ(qx, qy, qz, qw) { return }
        if tracks[entityID] == nil { tracks[entityID] = &Track{} }
        tracks[entityID].Frames++
    }
    for i := 16; i+28 <= len(data); i++ {
        if u32(i-8) == 0 && u32(i-4) == 0 {
            entityID := u32(i - 12)
            if entityID != 0 { addFrame(i-12, i, i+12) }
        }
    }
    known := map[uint32]bool{}
    for id := range tracks { known[id] = true }
    for i := 0; i+50 <= len(data); i++ {
        id := u32(i)
        if !known[id] || u32(i+4) != 0 || u32(i+8) == 0 { continue }
        addFrame(i, i+18, i+34)
    }
    bins := []int{1,2,3,4,5,10,20,50,100,500,1000}
    counts := map[int]int{}
    for _, t := range tracks {
        for _, b := range bins {
            if t.Frames >= b { counts[b]++ }
        }
    }
    for _, b := range bins { fmt.Printf("tracks_ge_%d=%d\n", b, counts[b]) }
}
