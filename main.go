package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/redraskal/r6-dissect/dissect"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var Version = "dev"

type OutputFormat = string

const (
	JSON  OutputFormat = "json"
	Excel OutputFormat = "excel"
)

func main() {
	setup()
	format := viper.GetString("format")
	in, err := viperFileOrDefault("input", os.Stdin, os.O_RDONLY)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	defer in.Close()
	out, err := viperFileOrDefault("output", os.Stdout, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	defer out.Close()
	stat, err := in.Stat()
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	if viper.GetBool("info") {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		if err := printHead(in); err != nil {
			log.Fatal().Err(err).Send()
		}
		return
	}
	if viper.GetBool("dump") && stat.IsDir() {
		log.Fatal().Msg("dump requires a replay file input.")
	}
	if viper.GetBool("dump") {
		if err := writeRoundDump(in, out); err != nil {
			log.Fatal().Err(err).Send()
		}
		return
	}
	if viper.GetBool("blender-camera") && stat.IsDir() {
		log.Fatal().Msg("blender-camera requires a replay file input.")
	}
	if viper.GetBool("blender-camera") {
		if err := writeRoundBlenderCamera(viper.GetString("input"), in, out); err != nil {
			log.Fatal().Err(err).Send()
		}
		return
	}
	if viper.GetBool("movement") {
		if stat.IsDir() {
			if err := writeMatchMovement(in, viper.GetString("output")); err != nil {
				log.Fatal().Err(err).Send()
			}
			return
		}
		if err := writeRoundMovement(viper.GetString("input"), in, out); err != nil {
			log.Fatal().Err(err).Send()
		}
		return
	}
	if viper.GetBool("movement-probe") && stat.IsDir() {
		log.Fatal().Msg("movement-probe requires a replay file input.")
	}
	if viper.GetBool("movement-probe") {
		if err := writeRoundMovementProbe(in, out); err != nil {
			log.Fatal().Err(err).Send()
		}
		return
	}
	if stat.IsDir() {
		if err := writeMatch(in, format, out); err != nil {
			log.Fatal().Err(err).Send()
		}
		return
	}
	if format == Excel {
		log.Fatal().Msg("Dissect will only export a match folder to Excel.")
	}
	if err = writeRound(in, out); err != nil {
		log.Fatal().Err(err).Send()
	}
}

func setup() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	pflag.StringP("format", "f", "", "specifies the output format (json, excel)")
	pflag.StringP("output", "o", "", "specifies the output path")
	pflag.BoolP("debug", "d", false, "sets log level to debug")
	pflag.BoolP("dump", "p", false, "dumps decompressed replay to the output")
	pflag.Bool("info", false, "prints the replay header")
	pflag.Bool("movement", false, "exports experimental movement/rotation tracks for a replay file")
	pflag.Bool("movement-probe", false, "exports movement prop/pair probe data for a replay file")
	pflag.Int("movement-workers", 0, "sets the worker count for batch movement folder export (0 = auto)")
	pflag.Bool("blender-camera", false, "exports a Blender Python camera importer for one replay track")
	pflag.String("position-prop", "", "overrides the auto-selected movement position prop id (24 hex chars)")
	pflag.String("rotation-prop", "", "overrides the auto-selected movement rotation prop id (24 hex chars)")
	pflag.String("camera-track", "", "selects the Blender camera track by guessed player name, label, or actor id")
	pflag.Int("camera-fps", 60, "sets the Blender scene FPS for camera export")
	pflag.Int("camera-frame-step", 1, "sets the frame step between exported movement samples")
	pflag.Float64("camera-scale", 1, "scales exported camera locations")
	pflag.String("camera-rotation-mode", "wrapped", "sets camera rotation mode: wrapped or raw")
	pflag.String("camera-yaw-axis", "z", "maps replay rotation to Blender yaw axis (x, y, z)")
	pflag.String("camera-pitch-axis", "x", "maps replay rotation to Blender pitch axis (x, y, z)")
	pflag.String("camera-roll-axis", "y", "maps replay rotation to Blender roll axis (x, y, z)")
	pflag.Float64("camera-yaw-sign", 1, "multiplies exported Blender yaw by this sign")
	pflag.Float64("camera-pitch-sign", 1, "multiplies exported Blender pitch by this sign")
	pflag.Float64("camera-roll-sign", 1, "multiplies exported Blender roll by this sign")
	pflag.Bool("camera-level", false, "exports a level camera when recovered heading is available by zeroing pitch/roll before applying offsets")
	pflag.Float64("camera-yaw-offset-deg", 0, "adds a constant yaw offset in degrees to the exported Blender camera")
	pflag.Float64("camera-pitch-offset-deg", 0, "adds a constant pitch offset in degrees to the exported Blender camera")
	pflag.Float64("camera-roll-offset-deg", 0, "adds a constant roll offset in degrees to the exported Blender camera")
	pflag.Bool("camera-compare-candidates", false, "exports alternate Blender camera rigs for macro-derived yaw/pitch candidate families")
	pflag.Int("camera-candidate-limit", 3, "sets the maximum number of alternate yaw and pitch candidate rigs to export in compare mode")
	pflag.BoolP("version", "v", false, "prints the version")
	pflag.Parse()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatal().Err(err)
	}
	if viper.GetBool("debug") {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}
	if viper.GetBool("version") {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		log.Info().Msgf("r6-dissect version: %s", Version)
		log.Info().Msg("https://github.com/redraskal/r6-dissect")
		os.Exit(0)
	}
	extra := len(pflag.Args())
	if extra < 1 && !piped(os.Stdin) {
		log.Fatal().Msg("Specify a valid match replay file/folder path (*.rec files)")
	} else if extra > 0 {
		viper.Set("input", pflag.Args()[0])
	}
	if !viper.IsSet("format") {
		output := viper.GetString("output")
		if strings.HasSuffix(output, ".xlsx") {
			viper.Set("format", "excel")
		} else if strings.HasSuffix(output, ".json") {
			viper.Set("format", "json")
		}
	}
	format := strings.ToLower(viper.GetString("format"))
	if len(format) > 0 && !(format == "json" || format == "excel") {
		log.Fatal().Msg("Specify a valid output format (json, excel)")
	} else if len(format) == 0 {
		viper.Set("format", "json")
	}
}

func printHead(in *os.File) error {
	stat, err := in.Stat()
	if err != nil {
		return err
	}
	if stat.IsDir() {
		m, err := dissect.NewMatchReader(in)
		if err != nil {
			return err
		}
		r, err := m.FirstRound()
		if err != nil {
			return err
		}
		r.Head()
		return nil
	}
	r, err := dissect.NewReader(in)
	if err != nil {
		return err
	}
	if err := r.ReadPartial(); !dissect.Ok(err) {
		return err
	}
	r.Head()
	return nil
}

func writeMatch(in *os.File, format OutputFormat, out io.Writer) error {
	m, err := dissect.NewMatchReader(in)
	if err != nil {
		return err
	}
	if err := m.Read(); !dissect.Ok(err) {
		return err
	}
	if format == Excel {
		return m.WriteExcel(out)
	}
	return m.WriteJSON(out)
}

func writeRound(in io.Reader, out io.Writer) error {
	r, err := dissect.NewReader(in)
	if err != nil {
		return err
	}
	type output struct {
		dissect.Header
		MatchFeedback []dissect.MatchUpdate      `json:"matchFeedback"`
		PlayerStats   []dissect.PlayerRoundStats `json:"stats"`
	}
	if err := r.Read(); !dissect.Ok(err) {
		return err
	}
	encoder := json.NewEncoder(out)
	return encoder.Encode(output{
		r.Header,
		r.MatchFeedback,
		r.PlayerStats(),
	})
}

func writeRoundMovement(inputPath string, in io.Reader, out io.Writer) error {
	r, err := dissect.NewReader(in)
	if err != nil {
		return err
	}
	data, err := r.MovementDataWithPlayersOptions(movementOptionsForInput(inputPath))
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(out)
	return encoder.Encode(data)
}

func writeMatchMovement(in *os.File, outputPath string) error {
	paths, err := dissect.ListReplayFiles(in)
	if err != nil {
		return err
	}
	outputDir := strings.TrimSpace(outputPath)
	if outputDir == "" {
		outputDir = in.Name()
	}
	info, err := os.Stat(outputDir)
	if err == nil && !info.IsDir() {
		return fmt.Errorf("movement batch output must be a directory: %s", outputDir)
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) {
		if mkdirErr := os.MkdirAll(outputDir, 0o755); mkdirErr != nil {
			return mkdirErr
		}
	}
	workers := movementWorkerCount(len(paths))
	type movementJob struct {
		index      int
		replayPath string
		outputPath string
	}
	type movementResult struct {
		index int
		err   error
	}
	jobs := make(chan movementJob)
	results := make(chan movementResult, len(paths))
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				results <- movementResult{
					index: job.index,
					err:   writeReplayMovementFile(job.replayPath, job.outputPath),
				}
			}
		}()
	}
	for index, replayPath := range paths {
		jobs <- movementJob{
			index:      index,
			replayPath: replayPath,
			outputPath: filepath.Join(outputDir, movementOutputName(replayPath)),
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	errs := make([]error, len(paths))
	for result := range results {
		errs[result.index] = result.err
	}
	for index, replayPath := range paths {
		if errs[index] != nil {
			return fmt.Errorf("%s: %w", replayPath, errs[index])
		}
	}
	return nil
}

func movementWorkerCount(jobCount int) int {
	if jobCount <= 0 {
		return 1
	}
	if workers := viper.GetInt("movement-workers"); workers > 0 {
		if workers > jobCount {
			return jobCount
		}
		return workers
	}
	workers := runtime.NumCPU() / 2
	if workers < 1 {
		workers = 1
	}
	if workers > 12 {
		workers = 12
	}
	if workers > jobCount {
		return jobCount
	}
	return workers
}

func writeReplayMovementFile(inputPath string, outputPath string) error {
	in, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer in.Close()
	tempPath := outputPath + ".tmp"
	out, err := os.Create(tempPath)
	if err != nil {
		return err
	}
	keepTemp := true
	defer func() {
		out.Close()
		if keepTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := writeRoundMovement(inputPath, in, out); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		return err
	}
	keepTemp = false
	return nil
}

func movementOutputName(inputPath string) string {
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	if ext == "" {
		return base + ".transforms.json"
	}
	return strings.TrimSuffix(base, ext) + ".transforms.json"
}

func writeRoundMovementProbe(in io.Reader, out io.Writer) error {
	r, err := dissect.NewReader(in)
	if err != nil {
		return err
	}
	data := r.MovementProbeOptions(movementOptionsFromFlags())
	encoder := json.NewEncoder(out)
	return encoder.Encode(data)
}

func writeRoundBlenderCamera(inputPath string, in io.Reader, out io.Writer) error {
	r, err := dissect.NewReader(in)
	if err != nil {
		return err
	}
	script, err := r.BlenderCameraScriptOptions(blenderCameraOptionsForInput(inputPath))
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, script)
	return err
}

func movementOptionsForInput(inputPath string) dissect.MovementOptions {
	return dissect.MovementOptions{
		PositionPropID: viper.GetString("position-prop"),
		RotationPropID: viper.GetString("rotation-prop"),
		SourcePath:     inputPath,
	}
}

func movementOptionsFromFlags() dissect.MovementOptions {
	return movementOptionsForInput(viper.GetString("input"))
}

func blenderCameraOptionsForInput(inputPath string) dissect.BlenderCameraOptions {
	return dissect.BlenderCameraOptions{
		MovementOptions:   movementOptionsForInput(inputPath),
		Track:             viper.GetString("camera-track"),
		FPS:               viper.GetInt("camera-fps"),
		FrameStep:         viper.GetInt("camera-frame-step"),
		Scale:             viper.GetFloat64("camera-scale"),
		RotationMode:      viper.GetString("camera-rotation-mode"),
		YawAxis:           viper.GetString("camera-yaw-axis"),
		PitchAxis:         viper.GetString("camera-pitch-axis"),
		RollAxis:          viper.GetString("camera-roll-axis"),
		YawSign:           viper.GetFloat64("camera-yaw-sign"),
		PitchSign:         viper.GetFloat64("camera-pitch-sign"),
		RollSign:          viper.GetFloat64("camera-roll-sign"),
		LevelCamera:       viper.GetBool("camera-level"),
		YawOffsetDeg:      viper.GetFloat64("camera-yaw-offset-deg"),
		PitchOffsetDeg:    viper.GetFloat64("camera-pitch-offset-deg"),
		RollOffsetDeg:     viper.GetFloat64("camera-roll-offset-deg"),
		CompareCandidates: viper.GetBool("camera-compare-candidates"),
		CandidateLimit:    viper.GetInt("camera-candidate-limit"),
	}
}

func blenderCameraOptionsFromFlags() dissect.BlenderCameraOptions {
	return blenderCameraOptionsForInput(viper.GetString("input"))
}

func writeRoundDump(in io.Reader, out *os.File) error {
	r, err := dissect.NewReader(in)
	if err != nil {
		return err
	}
	_, err = r.Write(out)
	return err
}

func piped(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return stat.Mode()&os.ModeNamedPipe != 0
}

func viperFileOrDefault(key string, def *os.File, flag int) (*os.File, error) {
	val := viper.GetString(key)
	if len(val) > 0 {
		return os.OpenFile(val, flag, os.ModePerm)
	}
	return def, nil
}
