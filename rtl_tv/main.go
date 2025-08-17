// NTSC receiver using github.com/jpoirier/gortlsdr (rtlsdr).
// Pipes decoded video to VLC for display.
// Copyright (c) 2025 SarahRoseLives - Updated by Gemini
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	rtl "github.com/jpoirier/gortlsdr"
)

// --- Configuration Constants ---
const (
	// Video parameters must match the transmitter
	FrameWidth  = 540
	FrameHeight = 480
	FrameRate   = 30000.0 / 1001.0

	// RTL-SDR parameters
	SampleRate = 2_000_000   // 2.4 MHz, must match transmitter's -rtl mode
	Frequency  = 427_250_000 // 427.25 MHz, target frequency
	RtlGain    = 350         // Max gain for RTL-SDR v3 (49.6 dB)
)

// --- Decoder ---
// The Decoder struct holds the state and logic for demodulating the NTSC signal.
type Decoder struct {
	// NTSC timing constants, calculated from the sample rate
	samplesPerLine    int
	activeStartSample int
	activeSamples     int

	// Frame buffer for the final RGB image sent to VLC
	frameBuffer []byte
	frameMutex  sync.Mutex

	// State variables for building the frame
	currentLine int

	// Simple AGC (Automatic Gain Control) levels
	// These are updated dynamically to adjust for signal strength changes.
	blankLevel float64 // Represents the NTSC black level
	peakLevel  float64 // Represents the NTSC white level
}

// NewDecoder initializes a new NTSC decoder with the correct timing constants.
func NewDecoder(sampleRate float64) *Decoder {
	d := &Decoder{}

	// An NTSC line has a duration of ~63.55 µs.
	// We calculate how many samples that corresponds to at our sample rate.
	lineDuration := 1.0 / (FrameRate * 525.0) // 525 lines per frame
	d.samplesPerLine = int(lineDuration * sampleRate)

	// Based on the transmitter's timings, we calculate when the visible
	// part of the video signal starts and how long it lasts, in samples.
	activeVideoStartTime := 10.7e-6 // 10.7 µs
	activeVideoDuration := 52.6e-6  // 52.6 µs
	d.activeStartSample = int(activeVideoStartTime * sampleRate)
	d.activeSamples = int(activeVideoDuration * sampleRate)

	// Allocate the buffer for one full frame of video (RGB24 format).
	d.frameBuffer = make([]byte, FrameWidth*FrameHeight*3)

	// Initialize AGC levels to reasonable defaults.
	d.blankLevel = 5000.0
	d.peakLevel = 15000.0

	return d
}

// processIQ is the core of the receiver. It takes a buffer of raw I/Q samples from the
// SDR, demodulates it, finds the sync pulses, and builds the video frame.
// It returns a complete frame when one is ready, otherwise it returns nil.
func (d *Decoder) processIQ(iq []byte) []byte {
	// 1. AM Demodulation
	// We convert the complex I/Q samples into a simple amplitude signal.
	// NTSC luminance is amplitude-modulated. We use magnitude squared to avoid
	// costly square root operations; the relative amplitudes are all that matter.
	amSignal := make([]float64, len(iq)/2)
	for i := 0; i < len(amSignal); i++ {
		iq_i := float64(int(iq[i*2]) - 127)
		iq_q := float64(int(iq[i*2+1]) - 127)
		amSignal[i] = (iq_i*iq_i + iq_q*iq_q)
	}

	// 2. Synchronization and Line Decoding
	// We scan through the demodulated signal, looking for horizontal sync pulses.
	samplePtr := 0
	for samplePtr < len(amSignal)-d.samplesPerLine {
		// Find the minimum value in a window the size of one line.
		// This minimum should be the tip of the HSYNC pulse.
		syncCandidatePos := -1
		minVal := 1e12 // Start with a very large number
		for i := 0; i < d.samplesPerLine; i++ {
			if amSignal[samplePtr+i] < minVal {
				minVal = amSignal[samplePtr+i]
				syncCandidatePos = samplePtr + i
			}
		}

		if syncCandidatePos == -1 {
			// Should not happen, but as a safeguard, we advance our pointer.
			samplePtr += d.samplesPerLine
			continue
		}

		// The start of the line is the HSYNC pulse position.
		lineStart := syncCandidatePos
		if lineStart+d.samplesPerLine > len(amSignal) {
			break // Not enough data left in this buffer for a full line.
		}

		// Extract the active video portion of this synchronized line.
		activeVideoStart := lineStart + d.activeStartSample
		if activeVideoStart+d.activeSamples > len(amSignal) {
			// Not enough data for the active video, so we'll skip this line
			// and search for the next sync pulse.
			samplePtr = lineStart + 1
			continue
		}
		lineSamples := amSignal[activeVideoStart : activeVideoStart+d.activeSamples]

		// 3. Simple AGC: Adjust black/white levels dynamically
		// We use the "back porch" (the period just after HSYNC) as our reference for black.
		// By assigning the result to a variable first, the conversion to int
		// becomes a runtime operation, where truncation is allowed.
		backPorchOffset := 5.6e-6 * float64(SampleRate)
		backPorchStart := lineStart + int(backPorchOffset)

		if backPorchStart < len(amSignal) {
			d.blankLevel = d.blankLevel*0.995 + amSignal[backPorchStart]*0.005
		}
		// Find the peak value in the active video to use as our white reference.
		maxInLine := 0.0
		for _, s := range lineSamples {
			if s > maxInLine {
				maxInLine = s
			}
		}
		d.peakLevel = d.peakLevel*0.995 + maxInLine*0.005

		// 4. Resampling and Pixel Generation
		// We map the ~126 active video samples to our 540 output pixels.
		if d.currentLine < FrameHeight {
			d.frameMutex.Lock()
			for pixelX := 0; pixelX < FrameWidth; pixelX++ {
				// Find the corresponding source sample for this destination pixel.
				srcIndex := int(float64(pixelX) / float64(FrameWidth) * float64(len(lineSamples)))

				// Normalize the sample's amplitude to a 0-255 grayscale value
				// using our dynamic black/white levels.
				levelRange := d.peakLevel - d.blankLevel
				if levelRange < 1.0 {
					levelRange = 1.0 // Avoid division by zero
				}
				normalizedVal := (lineSamples[srcIndex] - d.blankLevel) / levelRange
				gray := int(normalizedVal * 255.0)

				// Clamp the value to the valid 0-255 range.
				if gray < 0 {
					gray = 0
				}
				if gray > 255 {
					gray = 255
				}

				// Write the grayscale pixel to our frame buffer (R=G=B).
				offset := (d.currentLine*FrameWidth + pixelX) * 3
				d.frameBuffer[offset+0] = byte(gray)
				d.frameBuffer[offset+1] = byte(gray)
				d.frameBuffer[offset+2] = byte(gray)
			}
			d.frameMutex.Unlock()
		}

		// We've processed a line, move to the next one.
		d.currentLine++
		samplePtr = lineStart + d.samplesPerLine // Advance pointer to the next line

		// 5. Frame Completion
		// If we've filled all the lines, we have a complete frame.
		if d.currentLine >= FrameHeight {
			d.currentLine = 0 // Reset for the next frame
			d.frameMutex.Lock()
			frameCopy := make([]byte, len(d.frameBuffer))
			copy(frameCopy, d.frameBuffer) // Return a copy
			d.frameMutex.Unlock()
			return frameCopy
		}
	}

	// We processed the whole buffer but didn't complete a frame.
	return nil
}

// --- Main Application ---

// startVLCPipe launches VLC in a separate process, configured to display
// a raw RGB video stream from its standard input.
func startVLCPipe() (io.WriteCloser, *exec.Cmd, error) {
	vlcPath, err := exec.LookPath("vlc")
	if err != nil {
		return nil, nil, fmt.Errorf("VLC not found in your PATH. Please install VLC media player")
	}

	args := []string{
		"--demux", "rawvideo",
		"--rawvid-fps", fmt.Sprintf("%f", FrameRate),
		"--rawvid-width", fmt.Sprintf("%d", FrameWidth),
		"--rawvid-height", fmt.Sprintf("%d", FrameHeight),
		"--rawvid-chroma", "RV24", // RGB 24-bit
		"-", // Read from stdin
	}

	cmd := exec.Command(vlcPath, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = os.Stderr // Show VLC errors in the console

	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}

	log.Println("VLC process started. Video output should appear in a new window.")
	return stdinPipe, cmd, nil
}

func main() {
	log.Println("Starting RTL-SDR NTSC receiver...")

	// --- Initialize RTL-SDR Device ---
	devCount := rtl.GetDeviceCount()
	if devCount == 0 {
		log.Fatal("No RTL-SDR devices found.")
	}
	log.Printf("Found %d RTL-SDR device(s). Using device 0.\n", devCount)

	dongle, err := rtl.Open(0)
	if err != nil {
		log.Fatalf("Error opening RTL-SDR device: %v", err)
	}
	defer dongle.Close()

	// --- Configure Device Settings ---
	if err := dongle.SetCenterFreq(Frequency); err != nil {
		log.Fatalf("SetCenterFreq failed: %v", err)
	}
	log.Printf("Tuned to frequency: %.3f MHz\n", float64(Frequency)/1e6)

	if err := dongle.SetSampleRate(SampleRate); err != nil {
		log.Fatalf("SetSampleRate failed: %v", err)
	}
	log.Printf("Sample rate set to: %.3f MHz\n", float64(SampleRate)/1e6)

	// Set gain to a high value. You may need to adjust this.
	if err := dongle.SetTunerGainMode(false); err != nil { // false = manual gain
		log.Fatalf("SetTunerGainMode failed: %v", err)
	}
	if err := dongle.SetTunerGain(RtlGain); err != nil {
		log.Fatalf("SetTunerGain failed: %v", err)
	}
	log.Printf("Tuner gain set to: %.1f dB\n", float64(RtlGain)/10.0)

	if err := dongle.ResetBuffer(); err != nil {
		log.Fatalf("ResetBuffer failed: %v", err)
	}

	// --- Start VLC Pipe ---
	vlcPipe, vlcCmd, err := startVLCPipe()
	if err != nil {
		log.Fatalf("Failed to start VLC: %v", err)
	}
	defer vlcCmd.Process.Kill()
	defer vlcPipe.Close()

	// --- Main Processing Loop ---
	decoder := NewDecoder(SampleRate)
	log.Println("Starting stream processing. Looking for NTSC signal...")

	// Use the recommended buffer size for synchronous reading
	readBuffer := make([]byte, rtl.DefaultBufLength)

	for {
		bytesRead, err := dongle.ReadSync(readBuffer, len(readBuffer))
		if err != nil {
			log.Printf("ReadSync error: %v", err)
			break
		}

		if bytesRead != len(readBuffer) {
			log.Printf("Warning: short read (%d / %d bytes)", bytesRead, len(readBuffer))
			continue
		}

		// Process the received IQ data. This function will return a full frame
		// when one has been successfully decoded.
		frame := decoder.processIQ(readBuffer)
		if frame != nil {
			// We have a frame! Write it to VLC's stdin.
			if _, err := vlcPipe.Write(frame); err != nil {
				log.Println("Error writing to VLC pipe. VLC may have been closed.")
				break // Exit the loop if VLC is closed.
			}
		}
	}
}