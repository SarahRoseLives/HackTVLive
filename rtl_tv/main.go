package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rtltv/config"
	"rtltv/decoder"
	"rtltv/sdr"
	"rtltv/video"

	rtl "github.com/jpoirier/gortlsdr"
)

func main() {
	// 1. Configuration
	cfg := config.ParseFlags()
	log.Println("Starting RTL-SDR NTSC receiver...")

	// 2. Setup SDR Device
	dongle, err := sdr.SetupDevice(&cfg.SDR)
	if err != nil {
		log.Fatalf("SDR setup failed: %v", err)
	}
	defer dongle.Close()

	// 3. Setup Video Output
	ffplay, err := video.Start()
	if err != nil {
		log.Fatalf("Failed to start FFplay: %v", err)
	}
	defer ffplay.Stop()

	// 4. Initialize Decoder
	dec := decoder.New(float64(cfg.SDR.SampleRateHz))
	log.Println("Receiver started. Looking for NTSC sync pulses...")
	log.Printf("IMPORTANT: Transmitter must be running with matching -bw %.1f flag!", float64(cfg.SDR.SampleRateHz)/1e6)

	// 5. Start SDR Read Loop (in a separate goroutine)
	go func() {
		readBuffer := make([]byte, rtl.DefaultBufLength*2)
		for {
			bytesRead, err := dongle.ReadSync(readBuffer, len(readBuffer))
			if err != nil {
				log.Printf("SDR read loop stopped: %v", err)
				return
			}
			if bytesRead > 0 {
				dec.ProcessIQ(readBuffer[:bytesRead])
			}
		}
	}()

	// 6. Setup display ticker and graceful shutdown channel
	frameTicker := time.NewTicker(time.Second * 1001 / 30000) // Ticks at NTSC frame rate
	defer frameTicker.Stop()
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	log.Println("Application is running. Press Ctrl+C to exit.")

	// 7. Main loop to display frames and listen for shutdown
	for {
		select {
		case <-frameTicker.C:
			frame := dec.GetDisplayFrame()
			if _, err := ffplay.Pipe.Write(frame); err != nil {
				log.Println("Error writing to FFplay pipe, exiting. (Window was likely closed).")
				return
			}
		case <-shutdown:
			log.Println("Shutdown signal received, cleaning up...")
			return // Exit loop, allowing defers to run
		}
	}
}