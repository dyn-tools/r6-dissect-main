package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"github.com/redraskal/r6-dissect/dissect"
)

func main() {
	recPath := `e:\prog\R6Tools\Dissect Info\test_replays\Match-2026-03-09_01-49-33-28000\Match-2026-03-09_01-49-33-28000-R01.rec`

	f, err := os.Open(recPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	reader, err := dissect.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var buf bytes.Buffer
	if _, err := reader.Write(&buf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data := buf.Bytes()
	fmt.Printf("Decompressed: %d bytes\n\n", len(data))

	f32 := func(off int) float32 {
		if off < 0 || off+4 > len(data) {
			return float32(math.NaN())
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4]))
	}

	// Y-coordinate hits from float search (target ≈ 34.65, tolerance 0.2).
	// Treat as [X at yOff-4, Y at yOff, Z at yOff+4].
	// Dump header bytes before X to identify the packet structure.
	hits := []struct {
		yOff  int
		label string
	}{
		{22000, "spawn"},
		{23402, "2nd-frame"},
		{28471, "pre-macro-a"},
		{28919, "pre-macro-b"},
		{32790, "pre-macro-c"},
		{32953, "pre-macro-d"},
		{33062, "pre-macro-e"},
		{393457, "macro1-a"},
		{394711, "macro1-b"},
		{395965, "macro1-c"},
		{1866482, "macro2-a"},
		{1866946, "macro2-b"},
	}

	for _, h := range hits {
		xOff := h.yOff - 4
		x := f32(xOff)
		y := f32(h.yOff)
		z := f32(h.yOff + 4)
		fmt.Printf("=== %s  Y-off=0x%05X (%d) ===\n", h.label, h.yOff, h.yOff)
		fmt.Printf("  XYZ: %.4f  %.4f  %.4f\n", x, y, z)

		// Dump 56 bytes before X as hex
		start := xOff - 56
		if start < 0 {
			start = 0
		}
		fmt.Printf("  Header [0x%X .. 0x%X]:\n", start, xOff-1)
		for i := start; i < xOff; i++ {
			if (i-start)%16 == 0 {
				fmt.Printf("    %08X: ", i)
			}
			fmt.Printf("%02X ", data[i])
			if (i-start)%16 == 15 || i == xOff-1 {
				fmt.Println()
			}
		}

		// Show 20 bytes after Z (quaternion / extra)
		afterStart := h.yOff + 8
		fmt.Printf("  After-Z [0x%X]:", afterStart)
		for i := afterStart; i < afterStart+20 && i < len(data); i++ {
			fmt.Printf(" %02X", data[i])
		}
		fmt.Println()
		fmt.Println()
	}
}
