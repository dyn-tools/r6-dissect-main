package main

import (
    "encoding/binary"
    "encoding/json"
    "fmt"
    "math"
    "os"
    "sort"
)

type FriendTrack struct {
    EntityID  uint32
    EntityHex string
    Frames    int
}

type CurrentTrack struct {
    ActorID         string `json:"actorID"`
    PositionSamples int    `json:"positionSamples"`
    RotationSamples int    `json:"rotationSamples"`
}

type CurrentOutput struct {
    Tracks       []CurrentTrack `json:"tracks"`
    PrimaryTracks []CurrentTrack `json:"primaryTracks"`
}

func main() {
    data, err := os.ReadFile(`d:\r6-dissect-main\running_in_arcs_and_circles.dump.bin`)
    if err != nil { panic(err) }
    currentBytes, err := os.ReadFile(`d:\r6-dissect-main\replays\running_in_arcs_and_circles.current.json`)
    if err != nil { panic(err) }
    var current CurrentOutput
    if err := json.Unmarshal(currentBytes, &current); err != nil { panic(err) }

    f32 := func(off int) float32 {
        if off < 0 || off+4 > len(data) { return float32(math.NaN()) }
        return math.Float32frombits(binary.LittleEndian.Uint32(data[off:off+4]))
    }
    u32 := func(off int) uint32 {
        if off < 0 || off+4 > len(data) { return 0 }
        return binary.LittleEndian.Uint32(data[off:off+4])
    }
    isPlausibleCoord := func(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -2000 && v <= 2000 }
    isPlausibleHeight := func(v float32) bool { return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -100 && v <= 150 }
    isUnitQ := func(qx, qy, qz, qw float32) bool {
        for _, v := range []float32{qx, qy, qz, qw} {
            if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < -1.01 || v > 1.01 {
                return false
            }
        }
        sum := float64(qx*qx) + float64(qy*qy) + float64(qz*qz) + float64(qw*qw)
        return math.Abs(sum-1.0) < 0.05
    }

    trackMap := map[uint32]*FriendTrack{}
    order := make([]uint32, 0)
    addFrame := func(entityIDOff, xOff, quatOff int) {
        entityID := u32(entityIDOff)
        if entityID == 0 { return }
        x, y, z := f32(xOff), f32(xOff+4), f32(xOff+8)
        if !isPlausibleCoord(x) || !isPlausibleCoord(y) || !isPlausibleHeight(z) { return }
        if math.Abs(float64(x)) < 0.01 && math.Abs(float64(y)) < 0.01 && math.Abs(float64(z)) < 0.01 { return }
        qx, qy, qz, qw := f32(quatOff), f32(quatOff+4), f32(quatOff+8), f32(quatOff+12)
        if !isUnitQ(qx, qy, qz, qw) { return }
        track := trackMap[entityID]
        if track == nil {
            raw := data[entityIDOff : entityIDOff+4]
            track = &FriendTrack{EntityID: entityID, EntityHex: fmt.Sprintf("%02x %02x %02x %02x", raw[0], raw[1], raw[2], raw[3])}
            trackMap[entityID] = track
            order = append(order, entityID)
        }
        track.Frames++
    }

    for i := 16; i+28 <= len(data); i++ {
        if u32(i-8) == 0 && u32(i-4) == 0 {
            entityID := u32(i - 12)
            if entityID != 0 {
                addFrame(i-12, i, i+12)
            }
        }
    }
    knownIDs := map[uint32]bool{}
    for id := range trackMap { knownIDs[id] = true }
    for i := 0; i+50 <= len(data); i++ {
        entityID := u32(i)
        if !knownIDs[entityID] { continue }
        if u32(i+4) != 0 || u32(i+8) == 0 { continue }
        addFrame(i, i+18, i+34)
    }

    friendTracks := make([]FriendTrack, 0, len(order))
    for _, id := range order { friendTracks = append(friendTracks, *trackMap[id]) }
    sort.Slice(friendTracks, func(i, j int) bool { return friendTracks[i].Frames > friendTracks[j].Frames })
    sort.Slice(current.Tracks, func(i, j int) bool {
        li := current.Tracks[i].PositionSamples + current.Tracks[i].RotationSamples
        lj := current.Tracks[j].PositionSamples + current.Tracks[j].RotationSamples
        return li > lj
    })

    fmt.Printf("friend_track_count=%d\n", len(friendTracks))
    limit := 10
    if len(friendTracks) < limit { limit = len(friendTracks) }
    fmt.Printf("friend_top=")
    for i := 0; i < limit; i++ {
        if i > 0 { fmt.Print("; ") }
        fmt.Printf("%s:%d", friendTracks[i].EntityHex, friendTracks[i].Frames)
    }
    fmt.Println()
    fmt.Printf("current_track_count=%d\n", len(current.Tracks))
    fmt.Printf("current_primary_count=%d\n", len(current.PrimaryTracks))
    limit = 10
    if len(current.Tracks) < limit { limit = len(current.Tracks) }
    fmt.Printf("current_top=")
    for i := 0; i < limit; i++ {
        if i > 0 { fmt.Print("; ") }
        t := current.Tracks[i]
        fmt.Printf("%s:%dp/%dr", t.ActorID, t.PositionSamples, t.RotationSamples)
    }
    fmt.Println()
}
