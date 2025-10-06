package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/samuel/go-hackrf/hackrf"
	"hacktvlive/config"
	"hacktvlive/sdr"
	"hacktvlive/source"
	"hacktvlive/video"

)

func main() {
	cfg := config.New()

	// 1. Initialize HackRF and open device (Lifecycle is managed by main)
	if err := hackrf.Init(); err != nil {
		log.Fatalf("hackrf.Init() failed: %v", err)
	}
	defer hackrf.Exit()

	dev, err := hackrf.Open()
	if err != nil {
		log.Fatalf("hackrf.Open() failed: %v", err)
	}
	defer dev.Close()

	// 2. Select the video standard (NTSC or PAL)
	var videoStandard video.Standard
	var frameTick time.Duration
	if cfg.PAL {
		videoStandard = video.NewPAL(cfg.SampleRate)
		frameTick = time.Second / 25
	} else {
		videoStandard = video.NewNTSC(cfg.SampleRate)
		frameTick = time.Second * 1001 / 30000
	}

	// 3. Set up the video source (test pattern or FFmpeg)
	if cfg.Test {
		log.Println("Test mode: SMPTE color bars will be transmitted.")
		videoStandard.FillTestPattern()
		go func() {
			ticker := time.NewTicker(frameTick)
			defer ticker.Stop()
			for {
				<-ticker.C
				videoStandard.LockFrame()
				videoStandard.GenerateFullFrame()
				videoStandard.UnlockFrame()
			}
		}()
	} else {
		ffmpegCmd, err := source.StartFFmpegCapture(cfg, videoStandard)
		if err != nil {
			log.Fatalf("Failed to start video source: %v", err)
		}
		defer func() {
			if ffmpegCmd.Process != nil {
				_ = ffmpegCmd.Process.Kill()
			}
		}()
	}

	log.Println("Generating initial frame...")
	videoStandard.GenerateFullFrame()

	// 4. Start the SDR transmission using the opened device
	if err := sdr.Transmit(dev, cfg, videoStandard); err != nil {
		log.Fatalf("Transmission failed: %v", err)
	}

	// 5. Wait for a stop signal (Ctrl+C) to gracefully exit
	log.Println("Transmission is live. Press Ctrl+C to stop.")
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
}