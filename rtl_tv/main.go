package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"sync"

	rtl "github.com/jpoirier/gortlsdr"
)

// --- Configuration Constants ---
const (
	FrameWidth  = 540
	FrameHeight = 480
	FrameRate   = 30000.0 / 1001.0

	RtlGain = 496 // Max gain for RTL-SDR v3 (49.6 dB)
)

// --- Decoder ---
type Decoder struct {
	sampleRate            float64
	samplesPerLine        float64
	activeStartSample     int
	activeSamples         int
	hSyncWidthSamples     int
	vSyncMinWidthSamples  int
	frameBuffer           []byte
	frameMutex            sync.Mutex
	currentLine           int
	lastSyncPos           int
	whiteLevel            float64
	blankLevel            float64
	blackLevel            float64
	frameFound            bool
}

// NewDecoder initializes a new NTSC decoder with the correct timing constants.
func NewDecoder(sampleRate float64) *Decoder {
	d := &Decoder{}
	d.sampleRate = sampleRate
	d.lastSyncPos = -1 // Initialize PLL

	lineDuration := 1.0 / (FrameRate * 525.0)
	d.samplesPerLine = lineDuration * sampleRate

	activeVideoStartTime := 10.7e-6
	activeVideoDuration := 52.6e-6
	hSyncDuration := 4.7e-6
	vSyncDuration := 27.1e-6

	d.activeStartSample = int(activeVideoStartTime * sampleRate)
	d.activeSamples = int(activeVideoDuration * sampleRate)
	d.hSyncWidthSamples = int(hSyncDuration * sampleRate)
	d.vSyncMinWidthSamples = int(vSyncDuration * 0.8 * sampleRate)

	d.frameBuffer = make([]byte, FrameWidth*FrameHeight*3)

	d.whiteLevel = 50.0
	d.blankLevel = 100.0
	d.blackLevel = 150.0
	d.frameFound = false

	return d
}

// processIQ is the core of the receiver. It demodulates the signal, locks sync, and builds the video frame.
func (d *Decoder) processIQ(iq []byte) []byte {
	amSignal := make([]float64, len(iq)/2)
	for i := 0; i < len(amSignal); i++ {
		iq_i := float64(int(iq[i*2]) - 127)
		iq_q := float64(int(iq[i*2+1]) - 127)
		amSignal[i] = math.Sqrt(iq_i*iq_i + iq_q*iq_q)
	}

	samplePtr := 0
	samplesPerLineInt := int(d.samplesPerLine)

	for samplePtr < len(amSignal)-samplesPerLineInt {
		searchWindow := samplesPerLineInt
		if searchWindow > len(amSignal)-samplePtr {
			searchWindow = len(amSignal) - samplePtr
		}

		syncCandidatePos := -1
		maxVal := -1.0
		for i := 0; i < searchWindow; i++ {
			if amSignal[samplePtr+i] > maxVal {
				maxVal = amSignal[samplePtr+i]
				syncCandidatePos = samplePtr + i
			}
		}

		if d.lastSyncPos != -1 {
			detectedLineLen := syncCandidatePos - d.lastSyncPos
			if float64(detectedLineLen) > d.samplesPerLine*0.9 && float64(detectedLineLen) < d.samplesPerLine*1.1 {
				d.samplesPerLine = d.samplesPerLine*0.99 + float64(detectedLineLen)*0.01
			}
		}
		d.lastSyncPos = syncCandidatePos
		samplesPerLineInt = int(d.samplesPerLine)

		lineStart := syncCandidatePos
		if lineStart+samplesPerLineInt > len(amSignal) {
			break
		}

		syncThreshold := (d.blankLevel + d.blackLevel) / 2.0
		pulseWidth := 0
		for i := syncCandidatePos; i > syncCandidatePos-d.hSyncWidthSamples*2 && i > 0; i-- {
			if amSignal[i] < syncThreshold {
				break
			}
			pulseWidth++
		}
		for i := syncCandidatePos + 1; i < syncCandidatePos+d.hSyncWidthSamples*2 && i < len(amSignal); i++ {
			if amSignal[i] < syncThreshold {
				break
			}
			pulseWidth++
		}

		if pulseWidth >= d.vSyncMinWidthSamples {
			d.currentLine = 0
			samplePtr = lineStart + samplesPerLineInt
			continue
		}

		activeVideoStart := lineStart + d.activeStartSample
		if activeVideoStart+d.activeSamples > len(amSignal) {
			samplePtr = lineStart + 1
			continue
		}
		lineSamples := amSignal[activeVideoStart : activeVideoStart+d.activeSamples]

		backPorchOffset := 5.6e-6 * d.sampleRate
		backPorchStart := lineStart + int(backPorchOffset)
		if backPorchStart < len(amSignal) {
			d.blankLevel = d.blankLevel*0.995 + amSignal[backPorchStart]*0.005
		}
		maxInLine, minInLine := 0.0, 1e12
		for _, s := range lineSamples {
			if s > maxInLine {
				maxInLine = s
			}
			if s < minInLine {
				minInLine = s
			}
		}
		d.blackLevel = d.blackLevel*0.995 + maxInLine*0.005
		d.whiteLevel = d.whiteLevel*0.995 + minInLine*0.005

		if d.currentLine < FrameHeight {
			d.frameMutex.Lock()
			for pixelX := 0; pixelX < FrameWidth; pixelX++ {
				// Fix: Map every pixel to a sample, clamp to valid range
				srcIndex := int(float64(pixelX) * float64(len(lineSamples)) / float64(FrameWidth))
				if srcIndex >= len(lineSamples) {
					srcIndex = len(lineSamples) - 1
				}
				levelRange := d.blankLevel - d.whiteLevel
				if levelRange < 1.0 {
					levelRange = 1.0
				}
				normalizedVal := (d.blankLevel - lineSamples[srcIndex]) / levelRange
				gray := int(normalizedVal * 255.0)

				if gray < 0 {
					gray = 0
				}
				if gray > 255 {
					gray = 255
				}

				offset := (d.currentLine*FrameWidth + pixelX) * 3
				d.frameBuffer[offset+0] = byte(gray)
				d.frameBuffer[offset+1] = byte(gray)
				d.frameBuffer[offset+2] = byte(gray)
			}
			d.frameMutex.Unlock()
			d.frameFound = true
		}

		d.currentLine++
		samplePtr = lineStart + samplesPerLineInt

		if d.currentLine >= FrameHeight {
			d.currentLine = 0
			d.frameMutex.Lock()
			frameCopy := make([]byte, len(d.frameBuffer))
			copy(frameCopy, d.frameBuffer)
			d.frameMutex.Unlock()
			return frameCopy
		}
	}
	// If no frame detected, output static (gray noise)
	if !d.frameFound {
		frame := make([]byte, FrameWidth*FrameHeight*3)
		for i := 0; i < FrameWidth*FrameHeight; i++ {
			gray := byte(128 + int(32*math.Sin(float64(i))))
			frame[i*3+0] = gray
			frame[i*3+1] = gray
			frame[i*3+2] = gray
		}
		return frame
	}
	return nil
}

// --- Main Application ---

func startFFplayPipe() (io.WriteCloser, *exec.Cmd, error) {
	ffplayPath, err := exec.LookPath("ffplay")
	if err != nil {
		return nil, nil, fmt.Errorf("FFplay not found in your PATH")
	}
	args := []string{
		"-f", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", fmt.Sprintf("%dx%d", FrameWidth, FrameHeight),
		"-framerate", fmt.Sprintf("%f", FrameRate),
		"-i", "-",
	}
	cmd := exec.Command(ffplayPath, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	log.Println("FFplay process started. Video output should appear in a new window.")
	return stdinPipe, cmd, nil
}

func main() {
	bw := flag.Float64("bw", 2.4, "SDR sample rate (bandwidth) in MHz")
	freq := flag.Float64("freq", 427.25, "SDR center frequency in MHz")
	flag.Parse()

	sampleRateHz := *bw * 1_000_000
	frequencyHz := *freq * 1_000_000

	log.Println("Starting RTL-SDR NTSC receiver...")

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

	if err := dongle.SetCenterFreq(int(frequencyHz)); err != nil {
		log.Fatalf("SetCenterFreq failed: %v", err)
	}
	log.Printf("Tuned to frequency: %.3f MHz\n", frequencyHz/1e6)

	if err := dongle.SetSampleRate(int(sampleRateHz)); err != nil {
		log.Fatalf("SetSampleRate failed: %v", err)
	}
	log.Printf("Sample rate set to: %.3f MHz\n", sampleRateHz/1e6)

	if err := dongle.SetTunerGainMode(false); err != nil {
		log.Fatalf("SetTunerGainMode failed: %v", err)
	}
	if err := dongle.SetTunerGain(RtlGain); err != nil {
		log.Fatalf("SetTunerGain failed: %v", err)
	}
	log.Printf("Tuner gain set to: %.1f dB\n", float64(RtlGain)/10.0)

	if err := dongle.ResetBuffer(); err != nil {
		log.Fatalf("ResetBuffer failed: %v", err)
	}

	ffplayPipe, ffplayCmd, err := startFFplayPipe()
	if err != nil {
		log.Fatalf("Failed to start FFplay: %v", err)
	}
	defer ffplayCmd.Process.Kill()
	defer ffplayPipe.Close()

	decoder := NewDecoder(sampleRateHz)
	log.Println("Starting stream processing. Looking for NTSC signal...")

	readBuffer := make([]byte, rtl.DefaultBufLength)

	for {
		bytesRead, err := dongle.ReadSync(readBuffer, len(readBuffer))
		if err != nil {
			log.Printf("ReadSync error: %v", err)
			break
		}
		if bytesRead == 0 {
			continue
		}

		frame := decoder.processIQ(readBuffer[:bytesRead])
		if frame != nil {
			if _, err := ffplayPipe.Write(frame); err != nil {
				log.Println("Error writing to FFplay pipe. FFplay may have been closed.")
				break
			}
		}
	}
}