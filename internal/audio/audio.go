package audio

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Info holds basic probe data
type Info struct {
	Duration time.Duration
	Format   string
	BitRate  string
}

// Probe uses ffprobe to get duration, format and bitrate.
func Probe(ctx context.Context, inputPath string) (*Info, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration,format_name,bit_rate",
		"-of", "default=noprint_wrappers=1:nokey=0",
		inputPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}
	info := &Info{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "duration=") {
			val := strings.TrimPrefix(line, "duration=")
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				info.Duration = time.Duration(f * float64(time.Second))
			}
		} else if strings.HasPrefix(line, "format_name=") {
			info.Format = strings.TrimPrefix(line, "format_name=")
		} else if strings.HasPrefix(line, "bit_rate=") {
			info.BitRate = strings.TrimPrefix(line, "bit_rate=")
		}
	}
	return info, nil
}

// Transcode converts input to target outputPath with sane defaults.
// Eg. outputPath ends with .mp3 or .opus etc.
func Transcode(ctx context.Context, inputPath, outputPath string) error {
	// ensure parent exists when writing outside of storage wrapper (caller creates dir)
	// ffmpeg command:
	// -y overwrite, -i input, -vn no video, set sample rate/channels/bitrate
	args := []string{
		"-y",
		"-i", inputPath,
		"-vn",
		"-ar", "44100",
		"-ac", "2",
		"-b:a", "192k",
		outputPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	// capture stderr (ffmpeg prints progress there)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg transcode error: %w | stderr: %s", err, stderr.String())
	}
	return nil
}

// Loudness runs ffmpeg loudnorm analysis (two-pass style) to estimate integrated LUFS.
// This function runs ffmpeg with -af loudnorm=I=-16:TP=-1.5:LRA=11:print_format=summary
// and parses the printed stats. It returns integrated LUFS as a float (negative value).
func Loudness(ctx context.Context, inputPath string) (float64, error) {
	// Use ffmpeg to output loudnorm stats
	args := []string{
		"-hide_banner",
		"-i", inputPath,
		"-filter_complex", "loudnorm=I=-16:TP=-1.5:LRA=11:print_format=summary",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return 0, err
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	scanner := bufio.NewScanner(stderrPipe)
	var outLines []string
	for scanner.Scan() {
		l := scanner.Text()
		outLines = append(outLines, l)
	}
	_ = cmd.Wait()
	// join and parse for "Input Integrated:    -xx.xx LUFS" or "input_i"
	text := strings.Join(outLines, "\n")
	// Try to find "Input Integrated" pattern (human readable)
	re1 := regexp.MustCompile(`Input Integrated:\s*([-+]?\d+(\.\d+)?)\s*LUFS`)
	if m := re1.FindStringSubmatch(text); len(m) >= 2 {
		val, _ := strconv.ParseFloat(m[1], 64)
		return val, nil
	}
	// Try machine output fields like "input_i=-xx.xx"
	re2 := regexp.MustCompile(`input_i=([-+]?\d+(\.\d+)?)`)
	if m := re2.FindStringSubmatch(text); len(m) >= 2 {
		val, _ := strconv.ParseFloat(m[1], 64)
		return val, nil
	}
	return 0, errors.New("loudness not found")
}

// GenerateWaveform produces a PNG waveform using ffmpeg showwavespic filter.
// outputPNG must have .png extension.
func GenerateWaveform(ctx context.Context, inputPath, outputPNG string, width, height int) error {
	// ffmpeg -i input -filter_complex "aformat=channel_layouts=stereo,showwavespic=s=600x120" -frames:v 1 out.png
	size := fmt.Sprintf("%dx%d", width, height)
	args := []string{
		"-y",
		"-i", inputPath,
		"-filter_complex", fmt.Sprintf("aformat=channel_layouts=stereo,showwavespic=s=%s", size),
		"-frames:v", "1",
		outputPNG,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg waveform error: %w | stderr: %s", err, stderr.String())
	}
	return nil
}

// Helper: BuildOutputPath returns a safe output filename based on input and suffix.
func BuildOutputPath(baseDir, relPath, suffix string) string {
	ext := filepath.Ext(relPath)
	name := strings.TrimSuffix(relPath, ext)
	return filepath.Join(baseDir, name+suffix)
}
