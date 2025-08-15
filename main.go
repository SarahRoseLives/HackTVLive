package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
)

func getEnvInt(key string, def int) int {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return def
}

func main() {
	// Command-line flags
	freq := flag.Float64("freq", 0, "Transmit frequency in MHz (required)")
	audio := flag.Bool("audio", true, "Enable audio (default: true)")
	flag.Parse()

	// If no flags are provided, print instructions and exit
	if len(os.Args) == 1 {
		fmt.Println("No arguments provided.")
		fmt.Println("To use this transmitter, add the following RTMP URL as a custom output in OBS Studio:")
		fmt.Println("  rtmp://localhost:1935/live/stream")
		fmt.Println("Then run this program with:")
		fmt.Println("  --freq <frequency in MHz> [--audio=<true|false>]")
		fmt.Println("Example:")
		fmt.Println("  ./transmitter --freq 471.25 --audio=false")
		os.Exit(1)
	}

	// Check if frequency was provided
	if *freq == 0 {
		log.Fatalf("Error: --freq flag is required and must be a valid frequency in MHz.")
	}

	// Set up scale filter from environment or passthrough from OBS
	scaleWidth := os.Getenv("SCALE_WIDTH")
	scaleHeight := os.Getenv("SCALE_HEIGHT")
	var scaleArg string
	if scaleWidth != "" && scaleHeight != "" {
		scaleArg = "scale=" + scaleWidth + ":" + scaleHeight
		log.Printf("Using scale: %s", scaleArg)
	} else {
		scaleArg = "scale=iw:ih" // passthrough, uses input dimensions from OBS
		log.Printf("Using passthrough scale (input resolution from OBS)")
	}

	fps := os.Getenv("FPS")
	if fps == "" {
		fps = "15"
	}

	pixFmt := os.Getenv("PIX_FMT")
	if pixFmt == "" {
		pixFmt = "yuv420p"
	}

	// Prepare ffmpeg command: listen for RTMP, transcode to yuv4mpegpipe with dynamic scale
	ffmpegArgs := []string{
		"-listen", "1",
		"-i", "rtmp://0.0.0.0:1935/live/stream",
		"-vf", scaleArg,
		"-r", fps,
		"-pix_fmt", pixFmt,
		"-f", "yuv4mpegpipe", "-",
	}
	if !*audio {
		ffmpegArgs = append(ffmpegArgs, "-an") // disable audio
	}
	ffmpeg := exec.Command("ffmpeg", ffmpegArgs...)
	ffmpeg.Stderr = os.Stderr

	// Prepare hacktv command: read video from stdin, output NTSC analog signal
	hacktvArgs := []string{
		"-m", "m",
		"-f", fmt.Sprintf("%.0f000", *freq*1000), // Convert MHz to Hz and format
		"-s", "13500000",
		"-g", "40",
		"-", // read input from stdin
	}
	if !*audio {
		hacktvArgs = append(hacktvArgs, "--noaudio")
	}
	hacktv := exec.Command("./hacktv", hacktvArgs...)
	hacktv.Stdout = os.Stdout
	hacktv.Stderr = os.Stderr

	// Pipe ffmpeg's stdout to hacktv's stdin
	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to set up ffmpeg stdout pipe: %v", err)
	}
	hacktv.Stdin = ffmpegOut

	log.Println("Waiting for RTMP stream at rtmp://localhost:1935/live/stream...")

	// Start hacktv (must be running to accept pipe input)
	if err := hacktv.Start(); err != nil {
		log.Fatalf("Failed to start hacktv: %v", err)
	}
	// Start ffmpeg
	if err := ffmpeg.Start(); err != nil {
		log.Fatalf("Failed to start ffmpeg: %v", err)
	}
	// Wait for ffmpeg to finish
	if err := ffmpeg.Wait(); err != nil {
		log.Printf("ffmpeg exited with error: %v", err)
	}
	// Wait for hacktv to finish
	if err := hacktv.Wait(); err != nil {
		log.Printf("hacktv exited with error: %v", err)
	}
}