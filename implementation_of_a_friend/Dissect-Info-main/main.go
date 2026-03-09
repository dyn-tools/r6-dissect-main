package main

import (
	"bytes"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redraskal/r6-dissect/dissect"
)

//go:embed templates/index.html
var templateFS embed.FS

var tmpl *template.Template

// PageData is the root data passed to the HTML template.
type PageData struct {
	Error       string
	Mode        string // "single" | "match" | "analyze" | ""
	SingleRound *RoundView
	Match       *MatchView
	Analyze     *AnalyzeView
}

// ScoreboardEntry pairs a scoreboard record with a resolved username.
type ScoreboardEntry struct {
	Player   dissect.ScoreboardPlayer
	Username string
}

// RoundView holds all data extracted from a single *dissect.Reader.
type RoundView struct {
	Header               dissect.Header
	SourcePath           string
	HeaderOnly           bool
	RecordingPlayer      *dissect.Player
	RecordingPlayerIndex int
	NumPlayersTeam0      int
	NumPlayersTeam1      int
	OpeningKill          dissect.MatchUpdate
	OpeningDeath         dissect.MatchUpdate
	KillsAndDeaths       []dissect.MatchUpdate
	Trades               [][]dissect.MatchUpdate
	AllFeedback          []dissect.MatchUpdate
	Scoreboard           dissect.Scoreboard
	ScoreboardEntries    []ScoreboardEntry
	RoundStats           []dissect.PlayerRoundStats
	EntityTracks     []EntityTrack  // position/rotation extracted from binary
	EntityTracksJSON template.JS    // JSON-encoded for canvas visualization (marked safe)
}

// MatchView holds all data extracted from a *dissect.MatchReader.
type MatchView struct {
	NumRounds  int
	SourcePath string
	Rounds     []*RoundView
	MatchStats []dissect.PlayerMatchStats
	FirstRound *RoundView
	LastRound  *RoundView
}

// PosFrame is one position+rotation sample for an entity.
type PosFrame struct {
	Offset   int64
	EntityID uint32
	X, Y, Z  float32
	Qx, Qy   float32
	Qz, Qw   float32
	YawDeg   float32 // decoded from quaternion
}

// EntityTrack groups all position frames for one entity ID.
type EntityTrack struct {
	EntityID  uint32
	EntityHex string // "xx xx xx xx" display form
	Frames    []PosFrame
}

// AnalyzeView holds binary analysis results for a .rec file.
type AnalyzeView struct {
	SourcePath      string
	RawFileSize     int64
	DecompSize      int
	MapName         string
	GameVersion     string
	HexPreview      string // hex dump of first 256 bytes of decompressed data
	FloatTarget     string // user-supplied float to search for
	Tolerance       string
	HexPattern      string // user-supplied hex bytes to search for
	FloatHits       []FloatHit
	Vec3Hits        []Vec3Hit
	PatternContexts []PatternContext // bytes around known timer-tick markers
	HexPatternHits  []HexPatternHit // custom hex pattern search results
	StrideAnalysis  []StrideInfo    // most common Vec3 stride values
	EntityTracks    []EntityTrack   // position/rotation tracks per entity
	TruncatedAt     int
	Error           string
}

// PatternContext shows what surrounds a known binary marker in the replay.
type PatternContext struct {
	Offset   int64
	Before16 string
	After64  string
	AfterF32 []string // first 12 floats decoded from After64
}

// HexPatternHit is one occurrence of a user-specified hex byte sequence.
type HexPatternHit struct {
	Offset   int64
	Before16 string
	After48  string
	AfterF32 []string
}

// StrideInfo is a recurring distance between consecutive Vec3 hits.
type StrideInfo struct {
	Stride int
	Count  int
}

// FloatHit is one occurrence of a float value in the binary data.
type FloatHit struct {
	Offset  int64
	Value   float32
	Hex     string  // 4-byte LE hex
	Before8 string  // 8 bytes before
	After8  string  // 8 bytes after
	Prev    float32 // float32 at offset-4
	Next    float32 // float32 at offset+4
	Next2   float32 // float32 at offset+8
}

// Vec3Hit is a consecutive float32 triplet within map-plausible coordinate ranges.
type Vec3Hit struct {
	Offset int64
	X, Y, Z float32
}

func init() {
	funcMap := template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"derefBool": func(b *bool) string {
			if b == nil {
				return "—"
			}
			if *b {
				return "Yes"
			}
			return "No"
		},
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"formatFloat": func(f float64) string {
			return fmt.Sprintf("%.4f", f)
		},
		"formatFloat1": func(f float64) string {
			return fmt.Sprintf("%.1f", f)
		},
		"formatHex": func(b []byte) string {
			if len(b) == 0 {
				return "—"
			}
			return fmt.Sprintf("%x", b)
		},
		"formatF32": func(f float32) string {
			return fmt.Sprintf("%.4f", f)
		},
		"eventTypeName": func(t dissect.MatchUpdateType) string {
			switch t {
			case dissect.Kill:
				return "Kill"
			case dissect.Death:
				return "Death"
			case dissect.DefuserPlantStart:
				return "DefuserPlantStart"
			case dissect.DefuserPlantComplete:
				return "DefuserPlantComplete"
			case dissect.DefuserDisableStart:
				return "DefuserDisableStart"
			case dissect.DefuserDisableComplete:
				return "DefuserDisableComplete"
			case dissect.LocateObjective:
				return "LocateObjective"
			case dissect.OperatorSwap:
				return "OperatorSwap"
			case dissect.Battleye:
				return "Battleye"
			case dissect.PlayerLeave:
				return "PlayerLeave"
			default:
				return "Other"
			}
		},
		"eventTypeClass": func(t dissect.MatchUpdateType) string {
			switch t {
			case dissect.Kill:
				return "ev-kill"
			case dissect.Death:
				return "ev-death"
			case dissect.DefuserPlantStart, dissect.DefuserPlantComplete:
				return "ev-plant"
			case dissect.DefuserDisableStart, dissect.DefuserDisableComplete:
				return "ev-disable"
			case dissect.LocateObjective:
				return "ev-objective"
			case dissect.OperatorSwap:
				return "ev-swap"
			case dissect.Battleye:
				return "ev-battleye"
			case dissect.PlayerLeave:
				return "ev-leave"
			default:
				return "ev-other"
			}
		},
	}

	var err error
	tmpl, err = template.New("index.html").Funcs(funcMap).ParseFS(templateFS, "templates/index.html")
	if err != nil {
		log.Fatal("template parse error:", err)
	}
}

// recoverMiddleware catches panics from the dissect library (e.g. unknown operator IDs)
// and renders them as a user-visible error page instead of dropping the connection.
func recoverMiddleware(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("recovered panic: %v", rec)
				renderPage(w, PageData{Error: fmt.Sprintf("Library error (possibly unsupported game version / operator): %v", rec)})
			}
		}()
		h(w, r)
	}
}

func main() {
	addr := ":8080"

	// If server is already running, just open the browser and exit.
	if serverAlreadyRunning(addr) {
		log.Println("Server already running, opening browser...")
		openBrowser("http://localhost:8080")
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleIndex)
	mux.HandleFunc("POST /single", recoverMiddleware(handleSingle))
	mux.HandleFunc("POST /match", recoverMiddleware(handleMatch))
	mux.HandleFunc("POST /analyze", recoverMiddleware(handleAnalyze))
	mux.HandleFunc("POST /export/json/single", recoverMiddleware(handleExportJSONSingle))
	mux.HandleFunc("POST /export/json/match", recoverMiddleware(handleExportJSONMatch))
	mux.HandleFunc("POST /export/excel", recoverMiddleware(handleExportExcel))
	mux.HandleFunc("POST /export/raw", recoverMiddleware(handleExportRaw))

	// Open browser shortly after server starts.
	go func() {
		time.Sleep(300 * time.Millisecond)
		openBrowser("http://localhost:8080")
	}()

	log.Printf("R6 Dissect Info running at http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func serverAlreadyRunning(addr string) bool {
	conn, err := net.DialTimeout("tcp", "localhost"+addr, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func openBrowser(url string) {
	exec.Command("cmd", "/c", "start", "", url).Start()
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	renderPage(w, PageData{})
}

func handleSingle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		r.ParseForm()
	}

	headerOnly := r.FormValue("headeronly") == "on"
	path := r.FormValue("path")

	var src io.ReadCloser
	var sourcePath string

	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			renderPage(w, PageData{Error: fmt.Sprintf("Cannot open file: %v", err)})
			return
		}
		src = f
		sourcePath = path
	} else {
		file, _, err := r.FormFile("file")
		if err != nil {
			renderPage(w, PageData{Error: "Please provide a file path or upload a .rec file"})
			return
		}
		src = file
	}
	defer src.Close()

	reader, err := dissect.NewReader(src)
	if err != nil {
		renderPage(w, PageData{Error: fmt.Sprintf("Failed to create reader: %v", err)})
		return
	}

	// Capture decompressed bytes BEFORE Read() (which zeroes the internal buffer).
	var rawBuf bytes.Buffer
	reader.Write(&rawBuf)
	rawData := rawBuf.Bytes()

	if headerOnly {
		err = reader.ReadPartial()
	} else {
		err = reader.Read()
	}

	if !dissect.Ok(err) {
		renderPage(w, PageData{Error: fmt.Sprintf("Failed to read replay: %v", err)})
		return
	}

	rv := buildRoundView(reader, sourcePath, headerOnly)

	// Attach position/rotation tracks extracted from raw binary.
	if len(rawData) > 0 && !headerOnly {
		rv.EntityTracks = extractEntityPositions(rawData)
		if jb, jerr := json.Marshal(rv.EntityTracks); jerr == nil {
			rv.EntityTracksJSON = template.JS(jb)
		}
	}

	renderPage(w, PageData{Mode: "single", SingleRound: rv})
}

// matchRecFiles returns the sorted list of .rec file paths inside a match directory.
func matchRecFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".rec") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}

// attachEntityTracks opens a .rec file, decompresses it, extracts position tracks,
// and attaches them to the given RoundView. Safe to call even if the file can't be opened.
func attachEntityTracks(rv *RoundView, recPath string) {
	f, err := os.Open(recPath)
	if err != nil {
		return
	}
	defer f.Close()

	reader, err := dissect.NewReader(f)
	if err != nil {
		return
	}

	var rawBuf bytes.Buffer
	reader.Write(&rawBuf)
	rawData := rawBuf.Bytes()
	if len(rawData) == 0 {
		return
	}

	rv.EntityTracks = extractEntityPositions(rawData)
	if jb, jerr := json.Marshal(rv.EntityTracks); jerr == nil {
		rv.EntityTracksJSON = template.JS(jb)
	}
}

func handleMatch(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	path := r.FormValue("path")
	if path == "" {
		renderPage(w, PageData{Error: "Please provide a match path or prefix"})
		return
	}

	f, err := os.Open(path)
	if err != nil {
		renderPage(w, PageData{Error: fmt.Sprintf("Cannot open path: %v", err)})
		return
	}
	defer f.Close()

	mr, err := dissect.NewMatchReader(f)
	if err != nil {
		renderPage(w, PageData{Error: fmt.Sprintf("Failed to create match reader: %v", err)})
		return
	}

	if err := mr.Read(); !dissect.Ok(err) {
		renderPage(w, PageData{Error: fmt.Sprintf("Failed to read match: %v", err)})
		return
	}

	// Collect sorted .rec files from the match directory for position extraction.
	// mr.Read() zeroes all round buffers, so we must re-open each file independently.
	recFiles := matchRecFiles(path)

	n := mr.NumRounds()
	rounds := make([]*RoundView, 0, n)
	for i := 0; i < n; i++ {
		rd, err := mr.RoundAt(i)
		if err != nil && !dissect.Ok(err) {
			continue
		}
		if rd == nil {
			continue
		}
		rv := buildRoundView(rd, path, false)
		if i < len(recFiles) {
			attachEntityTracks(rv, recFiles[i])
		}
		rounds = append(rounds, rv)
	}

	var firstRound, lastRound *RoundView
	if fr, err := mr.FirstRound(); err == nil || dissect.Ok(err) {
		if fr != nil {
			firstRound = buildRoundView(fr, path, false)
			if len(recFiles) > 0 {
				attachEntityTracks(firstRound, recFiles[0])
			}
		}
	}
	if lr, err := mr.LastRound(); err == nil || dissect.Ok(err) {
		if lr != nil {
			lastRound = buildRoundView(lr, path, false)
			if len(recFiles) > 0 {
				attachEntityTracks(lastRound, recFiles[len(recFiles)-1])
			}
		}
	}

	mv := &MatchView{
		NumRounds:  n,
		SourcePath: path,
		Rounds:     rounds,
		MatchStats: mr.PlayerStats(),
		FirstRound: firstRound,
		LastRound:  lastRound,
	}
	renderPage(w, PageData{Mode: "match", Match: mv})
}

// handleAnalyze decompresses a .rec file and performs binary analysis:
// float value search, Vec3 scanning, pattern context, and stride detection.
func handleAnalyze(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	path := r.FormValue("path")
	floatTarget := strings.TrimSpace(r.FormValue("target"))
	toleranceStr := strings.TrimSpace(r.FormValue("tolerance"))
	doVec3 := r.FormValue("vec3") == "on"
	hexPattern := strings.TrimSpace(strings.ReplaceAll(r.FormValue("hexpattern"), " ", ""))

	if path == "" {
		renderPage(w, PageData{Mode: "analyze", Analyze: &AnalyzeView{Error: "Please provide a .rec file path"}})
		return
	}

	f, err := os.Open(path)
	if err != nil {
		renderPage(w, PageData{Mode: "analyze", Analyze: &AnalyzeView{Error: err.Error()}})
		return
	}
	defer f.Close()

	stat, _ := f.Stat()

	reader, err := dissect.NewReader(f)
	if err != nil {
		renderPage(w, PageData{Mode: "analyze", Analyze: &AnalyzeView{Error: fmt.Sprintf("NewReader failed: %v", err)}})
		return
	}

	// IMPORTANT: capture decompressed bytes BEFORE calling Read(),
	// because Read() sets r.b = nil when it finishes.
	var buf bytes.Buffer
	n, writeErr := reader.Write(&buf)
	if writeErr != nil || n == 0 {
		renderPage(w, PageData{Mode: "analyze", Analyze: &AnalyzeView{
			Error: fmt.Sprintf("Failed to capture decompressed data (wrote %d bytes): %v", n, writeErr),
		}})
		return
	}
	data := buf.Bytes()

	av := &AnalyzeView{
		SourcePath:  path,
		RawFileSize: stat.Size(),
		DecompSize:  len(data),
		MapName:     reader.Header.Map.String(),
		GameVersion: reader.Header.GameVersion,
		FloatTarget: floatTarget,
		Tolerance:   toleranceStr,
		HexPattern:  hexPattern,
	}

	previewLen := 256
	if len(data) < previewLen {
		previewLen = len(data)
	}
	av.HexPreview = buildHexDump(data[:previewLen])

	const maxResults = 500

	// --- Float value search ---
	if floatTarget != "" {
		target64, err := strconv.ParseFloat(floatTarget, 32)
		if err != nil {
			av.Error = fmt.Sprintf("Invalid float value %q: %v", floatTarget, err)
		} else {
			tolerance := float32(0.01)
			if toleranceStr != "" {
				if t, err := strconv.ParseFloat(toleranceStr, 32); err == nil {
					tolerance = float32(t)
				}
			}
			hits, truncated := findFloat(data, float32(target64), tolerance, maxResults)
			av.FloatHits = hits
			if truncated {
				av.TruncatedAt = maxResults
			}
		}
	}

	// --- Vec3 plausibility scan ---
	if doVec3 {
		hits, truncated := scanVec3(data, maxResults)
		av.Vec3Hits = hits
		if truncated && av.TruncatedAt == 0 {
			av.TruncatedAt = maxResults
		}
		// Stride analysis on Vec3 results
		if len(hits) > 1 {
			av.StrideAnalysis = analyzeStrides(hits, 20)
		}
	}

	// --- Known timer-pattern context (always run) ---
	// The library's time pattern fires every ~tick; surrounding bytes likely contain position.
	timerPattern := []byte{0x1F, 0x07, 0xEF, 0xC9}
	av.PatternContexts = findPatternContext(data, timerPattern, 50)

	// --- Custom hex pattern search ---
	if hexPattern != "" {
		patBytes, err := parseHexPattern(hexPattern)
		if err != nil {
			av.Error = fmt.Sprintf("Invalid hex pattern: %v", err)
		} else {
			hits := findHexPattern(data, patBytes, maxResults)
			av.HexPatternHits = hits
		}
	}

	// --- Position/rotation track extraction (always run) ---
	av.EntityTracks = extractEntityPositions(data)

	renderPage(w, PageData{Mode: "analyze", Analyze: av})
}

// extractEntityPositions scans the decompressed binary for position+rotation packets.
//
// Two packet types have been reverse-engineered (R6S Y11S1, .rec format):
//
// SPAWN packet (first appearance of entity):
//
//	[entity_id 4B] [00 00 00 00] [00 00 00 00] [X f32] [Y f32] [Z f32] [qx f32] [qy f32] [qz f32] [qw f32]
//
// FC-UPDATE packet (position update, flag byte 0xfc):
//
//	[entity_id 4B] [00 00 00 00] [counter 4B] [timestamp 4B] [0xfc flag_lo] [X f32] [Y f32] [Z f32] [extra 4B] [qx f32] [qy f32] [qz f32] [qw f32]
//
// Rotation encoding: yaw = 2 * atan2(qz, qw) * 180/π  (wraps; subtract 360 if > 180)
func extractEntityPositions(data []byte) []EntityTrack {
	f32 := func(off int) float32 {
		if off < 0 || off+4 > len(data) {
			return float32(math.NaN())
		}
		return math.Float32frombits(binary.LittleEndian.Uint32(data[off : off+4]))
	}
	u32 := func(off int) uint32 {
		if off < 0 || off+4 > len(data) {
			return 0
		}
		return binary.LittleEndian.Uint32(data[off : off+4])
	}
	isPlausibleCoord := func(v float32) bool {
		return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -2000 && v <= 2000
	}
	// Z is the vertical (up) axis in R6S — tighter height range for better false-positive rejection.
	isPlausibleHeight := func(v float32) bool {
		return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -100 && v <= 150
	}
	isUnitQ := func(qx, qy, qz, qw float32) bool {
		for _, v := range []float32{qx, qy, qz, qw} {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < -1.01 || v > 1.01 {
				return false
			}
		}
		sum := float64(qx)*float64(qx) + float64(qy)*float64(qy) + float64(qz)*float64(qz) + float64(qw)*float64(qw)
		return math.Abs(sum-1.0) < 0.05
	}
	calcYaw := func(qz, qw float32) float32 {
		return float32(2.0 * math.Atan2(float64(qz), float64(qw)) * 180.0 / math.Pi)
	}
	addFrame := func(trackMap map[uint32]*EntityTrack, order *[]uint32, data []byte, entityIDOff, xOff, quatOff int) {
		entityID := u32(entityIDOff)
		if entityID == 0 {
			return
		}
		x := f32(xOff); y := f32(xOff + 4); z := f32(xOff + 8)
		// X and Y are horizontal axes; Z is vertical (height) in R6S.
		if !isPlausibleCoord(x) || !isPlausibleCoord(y) || !isPlausibleHeight(z) {
			return
		}
		if math.Abs(float64(x)) < 0.01 && math.Abs(float64(y)) < 0.01 && math.Abs(float64(z)) < 0.01 {
			return
		}
		qx := f32(quatOff); qy := f32(quatOff + 4); qz := f32(quatOff + 8); qw := f32(quatOff + 12)
		if !isUnitQ(qx, qy, qz, qw) {
			return
		}
		pf := PosFrame{
			Offset:   int64(xOff),
			EntityID: entityID,
			X: x, Y: y, Z: z,
			Qx: qx, Qy: qy, Qz: qz, Qw: qw,
			YawDeg: calcYaw(qz, qw),
		}
		if _, ok := trackMap[entityID]; !ok {
			b := data[entityIDOff : entityIDOff+4]
			hex := fmt.Sprintf("%02x %02x %02x %02x", b[0], b[1], b[2], b[3])
			trackMap[entityID] = &EntityTrack{EntityID: entityID, EntityHex: hex}
			*order = append(*order, entityID)
		}
		trackMap[entityID].Frames = append(trackMap[entityID].Frames, pf)
	}

	trackMap := make(map[uint32]*EntityTrack)
	var order []uint32

	// Pass 1: SPAWN packets — [entity_id 4B][null 4B][null 4B][X f32][Y f32][Z f32][qx qy qz qw]
	// Detection: 8 null bytes immediately before X; entity_id at X-12; quaternion at X+12.
	for i := 16; i+28 <= len(data); i++ {
		if u32(i-8) == 0 && u32(i-4) == 0 {
			entityID := u32(i - 12)
			if entityID != 0 {
				addFrame(trackMap, &order, data, i-12, i, i+12)
			}
		}
	}

	// Build set of entity IDs confirmed by spawn packets.
	knownIDs := make(map[uint32]bool, len(trackMap))
	for id := range trackMap {
		knownIDs[id] = true
	}

	// Pass 2: FC-UPDATE packets — [entity_id 4B][null 4B][counter 4B][timestamp 4B][flag 2B][X f32][Y f32][Z f32][extra 4B][qx qy qz qw]
	// Detection: null at +4, non-zero counter at +8; X at +18, quaternion at +34.
	// No flag-byte filter: sprint/walk/crouch/prone/lean may produce different flag values.
	// Only accept entity IDs already seen in pass 1 to avoid false positives.
	for i := 0; i+50 <= len(data); i++ {
		entityID := u32(i)
		if !knownIDs[entityID] {
			continue
		}
		if u32(i+4) != 0 || u32(i+8) == 0 {
			continue
		}
		addFrame(trackMap, &order, data, i, i+18, i+34)
	}

	result := make([]EntityTrack, 0, len(order))
	for _, id := range order {
		result = append(result, *trackMap[id])
	}
	return result
}

// findFloat searches data for float32 values matching target ± tolerance.
func findFloat(data []byte, target, tolerance float32, maxHits int) ([]FloatHit, bool) {
	var hits []FloatHit
	for i := 0; i <= len(data)-4; i++ {
		bits := binary.LittleEndian.Uint32(data[i : i+4])
		val := math.Float32frombits(bits)
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			continue
		}
		if float32(math.Abs(float64(val-target))) > tolerance {
			continue
		}
		hit := FloatHit{
			Offset: int64(i),
			Value:  val,
			Hex:    fmt.Sprintf("%08x", bits),
		}
		start := i - 8
		if start < 0 {
			start = 0
		}
		hit.Before8 = fmt.Sprintf("%x", data[start:i])
		end := i + 12
		if end > len(data) {
			end = len(data)
		}
		hit.After8 = fmt.Sprintf("%x", data[i+4:end])
		if i >= 4 {
			hit.Prev = math.Float32frombits(binary.LittleEndian.Uint32(data[i-4 : i]))
		}
		if i+8 <= len(data) {
			hit.Next = math.Float32frombits(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		}
		if i+12 <= len(data) {
			hit.Next2 = math.Float32frombits(binary.LittleEndian.Uint32(data[i+8 : i+12]))
		}
		hits = append(hits, hit)
		if len(hits) >= maxHits {
			return hits, true
		}
	}
	return hits, false
}

// scanVec3 finds consecutive float32 triplets within R6-plausible map coordinate ranges.
func scanVec3(data []byte, maxHits int) ([]Vec3Hit, bool) {
	var hits []Vec3Hit
	isValidXZ := func(v float32) bool {
		return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -1500 && v <= 1500
	}
	isValidY := func(v float32) bool {
		return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) && v >= -150 && v <= 300
	}
	for i := 0; i <= len(data)-12; i += 4 {
		x := math.Float32frombits(binary.LittleEndian.Uint32(data[i : i+4]))
		y := math.Float32frombits(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		z := math.Float32frombits(binary.LittleEndian.Uint32(data[i+8 : i+12]))
		if !isValidXZ(x) || !isValidY(y) || !isValidXZ(z) {
			continue
		}
		// Skip all-zero or near-zero triplets (very common false positive)
		if math.Abs(float64(x)) < 0.5 && math.Abs(float64(y)) < 0.5 && math.Abs(float64(z)) < 0.5 {
			continue
		}
		hits = append(hits, Vec3Hit{Offset: int64(i), X: x, Y: y, Z: z})
		if len(hits) >= maxHits {
			return hits, true
		}
	}
	return hits, false
}

// buildHexDump produces a classic hex dump string of the given bytes.
func buildHexDump(data []byte) string {
	var sb strings.Builder
	for i := 0; i < len(data); i += 16 {
		end := i + 16
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		fmt.Fprintf(&sb, "%08x  ", i)
		for j, b := range chunk {
			fmt.Fprintf(&sb, "%02x ", b)
			if j == 7 {
				sb.WriteString(" ")
			}
		}
		for j := len(chunk); j < 16; j++ {
			sb.WriteString("   ")
			if j == 7 {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(" |")
		for _, b := range chunk {
			if b >= 32 && b < 127 {
				sb.WriteByte(b)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}

// findPatternContext scans data for a byte pattern and returns surrounding context.
func findPatternContext(data, pattern []byte, maxHits int) []PatternContext {
	var results []PatternContext
	pLen := len(pattern)
	for i := 0; i <= len(data)-pLen; i++ {
		if !bytes.Equal(data[i:i+pLen], pattern) {
			continue
		}
		pc := PatternContext{Offset: int64(i)}
		start := i - 16
		if start < 0 {
			start = 0
		}
		pc.Before16 = fmt.Sprintf("%x", data[start:i])
		end := i + pLen + 64
		if end > len(data) {
			end = len(data)
		}
		after := data[i+pLen : end]
		pc.After64 = fmt.Sprintf("%x", after)
		// Decode up to 12 float32s from the bytes after the pattern
		for j := 0; j+4 <= len(after) && len(pc.AfterF32) < 12; j += 4 {
			v := math.Float32frombits(binary.LittleEndian.Uint32(after[j : j+4]))
			if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) {
				pc.AfterF32 = append(pc.AfterF32, fmt.Sprintf("%.4f", v))
			} else {
				pc.AfterF32 = append(pc.AfterF32, "NaN/Inf")
			}
		}
		results = append(results, pc)
		if len(results) >= maxHits {
			break
		}
	}
	return results
}

// parseHexPattern converts a hex string (e.g. "1f07efc9" or "1F 07 EF C9") into bytes.
func parseHexPattern(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "0x", "")
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("odd number of hex digits")
	}
	b := make([]byte, len(s)/2)
	for i := range b {
		_, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &b[i])
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

// findHexPattern finds all occurrences of a byte sequence in data.
func findHexPattern(data, pattern []byte, maxHits int) []HexPatternHit {
	var hits []HexPatternHit
	pLen := len(pattern)
	for i := 0; i <= len(data)-pLen; i++ {
		if !bytes.Equal(data[i:i+pLen], pattern) {
			continue
		}
		hit := HexPatternHit{Offset: int64(i)}
		start := i - 16
		if start < 0 {
			start = 0
		}
		hit.Before16 = fmt.Sprintf("%x", data[start:i])
		end := i + pLen + 48
		if end > len(data) {
			end = len(data)
		}
		after := data[i+pLen : end]
		hit.After48 = fmt.Sprintf("%x", after)
		for j := 0; j+4 <= len(after) && len(hit.AfterF32) < 10; j += 4 {
			v := math.Float32frombits(binary.LittleEndian.Uint32(after[j : j+4]))
			if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) {
				hit.AfterF32 = append(hit.AfterF32, fmt.Sprintf("%.4f", v))
			} else {
				hit.AfterF32 = append(hit.AfterF32, "?")
			}
		}
		hits = append(hits, hit)
		if len(hits) >= maxHits {
			break
		}
	}
	return hits
}

// analyzeStrides finds the most common byte distances between consecutive Vec3 hits.
func analyzeStrides(hits []Vec3Hit, topN int) []StrideInfo {
	counts := make(map[int]int)
	for i := 1; i < len(hits); i++ {
		d := int(hits[i].Offset - hits[i-1].Offset)
		if d > 0 && d < 4096 { // ignore huge gaps
			counts[d]++
		}
	}
	type kv struct{ k, v int }
	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	// Sort descending by count
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].v > sorted[i].v {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	if topN > len(sorted) {
		topN = len(sorted)
	}
	result := make([]StrideInfo, topN)
	for i := range result {
		result[i] = StrideInfo{Stride: sorted[i].k, Count: sorted[i].v}
	}
	return result
}

func handleExportJSONSingle(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "no path provided (upload-only files cannot be exported)", http.StatusBadRequest)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()

	reader, err := dissect.NewReader(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := reader.Read(); !dissect.Ok(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="round.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(map[string]any{
		"header":          reader.Header,
		"matchFeedback":   reader.MatchFeedback,
		"scoreboard":      reader.Scoreboard,
		"playerStats":     reader.PlayerStats(),
		"openingKill":     reader.OpeningKill(),
		"openingDeath":    reader.OpeningDeath(),
		"killsAndDeaths":  reader.KillsAndDeaths(),
		"trades":          reader.Trades(),
		"numPlayersTeam0": reader.NumPlayers(0),
		"numPlayersTeam1": reader.NumPlayers(1),
	})
}

func handleExportJSONMatch(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "no path", http.StatusBadRequest)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()

	mr, err := dissect.NewMatchReader(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mr.Read(); !dissect.Ok(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="match.json"`)
	mr.WriteJSON(w)
}

func handleExportExcel(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "no path", http.StatusBadRequest)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()

	mr, err := dissect.NewMatchReader(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mr.Read(); !dissect.Ok(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="match.xlsx"`)
	mr.WriteExcel(w)
}

func handleExportRaw(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	path := r.FormValue("path")
	if path == "" {
		http.Error(w, "no path", http.StatusBadRequest)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()

	reader, err := dissect.NewReader(f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Write decompressed bytes BEFORE Read() (which nilifies the buffer).
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="round_decompressed.bin"`)
	reader.Write(w)
}

// buildRoundView calls every Reader method and populates a RoundView.
func buildRoundView(rd *dissect.Reader, sourcePath string, headerOnly bool) *RoundView {
	rv := &RoundView{
		Header:          rd.Header,
		SourcePath:      sourcePath,
		HeaderOnly:      headerOnly,
		NumPlayersTeam0: rd.NumPlayers(0),
		NumPlayersTeam1: rd.NumPlayers(1),
	}

	// Find recording player by matching RecordingPlayerID -> Player.ID
	for i := range rd.Header.Players {
		if rd.Header.Players[i].ID == rd.Header.RecordingPlayerID {
			p := rd.Header.Players[i]
			rv.RecordingPlayer = &p
			break
		}
	}

	// Demonstrate PlayerIndexByUsername
	if rv.RecordingPlayer != nil {
		rv.RecordingPlayerIndex = rd.PlayerIndexByUsername(rv.RecordingPlayer.Username)
	}

	if !headerOnly {
		rv.OpeningKill = rd.OpeningKill()
		rv.OpeningDeath = rd.OpeningDeath()
		rv.KillsAndDeaths = rd.KillsAndDeaths()
		rv.Trades = rd.Trades()
		rv.AllFeedback = rd.MatchFeedback
		rv.Scoreboard = rd.Scoreboard
		rv.RoundStats = rd.PlayerStats()

		// Build scoreboard entries using PlayerIndexByID to resolve usernames
		rv.ScoreboardEntries = make([]ScoreboardEntry, 0, len(rd.Scoreboard.Players))
		for _, sp := range rd.Scoreboard.Players {
			username := ""
			idx := rd.PlayerIndexByID(sp.ID)
			if idx >= 0 && idx < len(rd.Header.Players) {
				username = rd.Header.Players[idx].Username
			}
			rv.ScoreboardEntries = append(rv.ScoreboardEntries, ScoreboardEntry{
				Player:   sp,
				Username: username,
			})
		}
	}

	return rv
}

func renderPage(w http.ResponseWriter, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("template error: %v", err)
	}
}
