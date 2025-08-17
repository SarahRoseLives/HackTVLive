package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/samuel/go-hackrf/hackrf"
)

// Video source resolution we will ask FFmpeg to produce
const (
	FrameWidth  = 540
	FrameHeight = 480
)

// NTSC struct holds all constants and state for generating the NTSC signal.
type NTSC struct {
	sampleRate         float64
	frameRate          float64
	linesPerFrame      int
	activeVideoLines   int
	lineSamples        int
	hSyncSamples       int
	vSyncPulseSamples  int
	eqPulseSamples     int
	burstStartSamples  int
	burstEndSamples    int
	activeStartSamples int
	activeSamples      int
	fsc                float64
	levelSync          float64
	levelBlanking      float64
	levelBlack         float64
	levelWhite         float64
	burstAmplitude     float64
	rawFrameBuffer     []byte
	rawFrameMutex      sync.RWMutex
	ntscFrameBuffer    []float64
	ntscFrameMutex     sync.RWMutex
}

func NewNTSC(sampleRate float64) *NTSC {
	n := &NTSC{
		sampleRate:       sampleRate,
		frameRate:        30000.0 / 1001.0,
		linesPerFrame:    525,
		activeVideoLines: 480,
		fsc:              3579545.4545,
		levelSync:        -40.0,
		levelBlanking:    0.0,
		levelBlack:       7.5,
		levelWhite:       100.0,
		burstAmplitude:   20.0,
	}
	lineDuration := 1.0 / (n.frameRate * float64(n.linesPerFrame))
	n.lineSamples = int(lineDuration * n.sampleRate)
	n.hSyncSamples = int(4.7e-6 * n.sampleRate)
	n.vSyncPulseSamples = int(27.1e-6 * n.sampleRate)
	n.eqPulseSamples = int(2.3e-6 * n.sampleRate)
	n.burstStartSamples = int(5.6e-6 * n.sampleRate)
	n.burstEndSamples = n.burstStartSamples + int(2.5e-6*n.sampleRate)
	n.activeStartSamples = int(10.7e-6 * n.sampleRate)
	n.activeSamples = int(52.6e-6 * n.sampleRate)
	n.rawFrameBuffer = make([]byte, FrameWidth*FrameHeight*3)
	n.ntscFrameBuffer = make([]float64, n.lineSamples*n.linesPerFrame)
	return n
}

func (n *NTSC) GenerateFullFrame() {
	var subcarrierPhase float64 = 0.0
	phaseIncrement := 2.0 * math.Pi * n.fsc / n.sampleRate
	for line := 1; line <= n.linesPerFrame; line++ {
		lineBuffer := n.generateLumaLine(line)
		isVBI := (line >= 1 && line <= 21) || (line >= 264 && line <= 284)
		if !isVBI {
			n.rawFrameMutex.RLock()
			for s := 0; s < n.lineSamples; s++ {
				if s >= n.burstStartSamples && s < n.burstEndSamples {
					lineBuffer[s] += n.burstAmplitude * math.Sin(subcarrierPhase+math.Pi)
				} else if s >= n.activeStartSamples && s < (n.activeStartSamples+n.activeSamples) {
					_, i, q := n.getPixelYIQ(line, s)
					lineBuffer[s] += i*math.Cos(subcarrierPhase) + q*math.Sin(subcarrierPhase)
				}
				subcarrierPhase += phaseIncrement
			}
			n.rawFrameMutex.RUnlock()
		} else {
			subcarrierPhase += phaseIncrement * float64(n.lineSamples)
		}
		offset := (line - 1) * n.lineSamples
		copy(n.ntscFrameBuffer[offset:], lineBuffer)
	}
}

func (n *NTSC) getPixelYIQ(currentLine, sampleInLine int) (y, i, q float64) {
	videoLine := 0
	if currentLine >= 22 && currentLine <= 263 {
		videoLine = (currentLine - 22) * 2
	} else if currentLine >= 285 && currentLine <= 525 {
		videoLine = (currentLine - 285) * 2 + 1
	}
	sampleInActiveVideo := sampleInLine - n.activeStartSamples
	pixelX := int(float64(sampleInActiveVideo) / float64(n.activeSamples) * FrameWidth)
	if videoLine < 0 || videoLine >= FrameHeight || pixelX < 0 || pixelX >= FrameWidth {
		return n.levelBlack, 0, 0
	}
	n.rawFrameMutex.RLock()
	pixelIndex := (videoLine*FrameWidth + pixelX) * 3
	r := float64(n.rawFrameBuffer[pixelIndex])
	g := float64(n.rawFrameBuffer[pixelIndex+1])
	b := float64(n.rawFrameBuffer[pixelIndex+2])
	n.rawFrameMutex.RUnlock()
	yVal := 0.299*r + 0.587*g + 0.114*b
	iVal := 0.596*r - 0.274*g - 0.322*b
	qVal := 0.211*r - 0.523*g + 0.312*b
	y = n.levelBlack + yVal/255.0*(n.levelWhite-n.levelBlack)
	i = iVal / 255.0 * (n.levelWhite - n.levelBlack)
	q = qVal / 255.0 * (n.levelWhite - n.levelBlack)
	return
}

func (n *NTSC) generateLumaLine(currentLine int) []float64 {
	lineBuffer := make([]float64, n.lineSamples)
	for s := 0; s < n.lineSamples; s++ {
		lineBuffer[s] = n.levelBlanking
	}
	lineInField := currentLine
	if currentLine > n.linesPerFrame/2 {
		lineInField = currentLine - (n.linesPerFrame / 2)
	}
	isVBI := lineInField <= 21
	halfLine := n.lineSamples / 2
	switch {
	case lineInField >= 1 && lineInField <= 3:
		for s := 0; s < n.eqPulseSamples; s++ {
			lineBuffer[s], lineBuffer[halfLine+s] = n.levelSync, n.levelSync
		}
		return lineBuffer
	case lineInField >= 4 && lineInField <= 6:
		for s := 0; s < n.vSyncPulseSamples; s++ {
			lineBuffer[s], lineBuffer[halfLine+s] = n.levelSync, n.levelSync
		}
		return lineBuffer
	case lineInField >= 7 && lineInField <= 9:
		for s := 0; s < n.eqPulseSamples; s++ {
			lineBuffer[s], lineBuffer[halfLine+s] = n.levelSync, n.levelSync
		}
		return lineBuffer
	}
	for s := 0; s < n.hSyncSamples; s++ {
		lineBuffer[s] = n.levelSync
	}
	if !isVBI {
		n.rawFrameMutex.RLock()
		for s := 0; s < n.activeSamples; s++ {
			y, _, _ := n.getPixelYIQ(currentLine, n.activeStartSamples+s)
			lineBuffer[n.activeStartSamples+s] = y
		}
		n.rawFrameMutex.RUnlock()
	}
	return lineBuffer
}

func (n *NTSC) ireToAmplitude(ire float64) float64 {
	return ((ire - 100.0) / -140.0) * (1.0 - 0.125) + 0.125
}

func main() {
	// --- Command-Line Flag Setup ---
	freq := flag.Float64("freq", 0, "Transmit frequency in MHz (required)")
	sampleRate := flag.Int("samplerate", 8000000, "Sample rate in Hz")
	gain := flag.Int("gain", 47, "TX VGA gain (0-47)")
	device := flag.String("device", "", "Video device name or index (OS-dependent, see instructions)")
	callsign := flag.String("callsign", "", "Callsign to overlay on the video (optional)")
	rtl := flag.Bool("rtl", false, "Enable 2.4 MHz RTL mode (signal output at 2.4 MHz)")
	flag.Parse()

	if *freq == 0 {
		fmt.Println("Usage: ./tvtx -freq <mhz> -device <name_or_index> [-callsign <callsign>] [-rtl]")
		fmt.Println("\nThis program captures a webcam and transmits it as NTSC video.")
		fmt.Println("\nStep 1: Find your webcam's name or index (see README).")
		fmt.Println("Step 2: Run this program with the correct arguments.")
		fmt.Println("\nExample (Linux):    ./tvtx -freq 427.25 -device /dev/video0 -callsign N0CALL")
		fmt.Println("Example (macOS):    ./tvtx -freq 427.25 -device 0 -callsign N0CALL")
		fmt.Println("Example (Windows):  ./tvtx -freq 427.25 -device \"Integrated Webcam\" -callsign N0CALL")
		fmt.Println("Example (RTL mode): ./tvtx -freq 427.25 -device /dev/video0 -rtl")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// --- Build the FFmpeg command based on the operating system ---
	var ffmpegArgs []string
	switch runtime.GOOS {
	case "linux":
		dev := *device
		if dev == "" {
			dev = "/dev/video0"
		}
		ffmpegArgs = []string{
			"-f", "v4l2", "-i", dev,
		}
	case "darwin": // macOS
		dev := *device
		if dev == "" {
			dev = "0"
		}
		ffmpegArgs = []string{
			"-f", "avfoundation", "-i", dev,
		}
	case "windows":
		dev := *device
		if dev == "" {
			dev = "Integrated Webcam" // A common default
		}
		ffmpegArgs = []string{
			"-f", "dshow", "-i", "video=" + dev,
		}
	default:
		log.Fatalf("Unsupported OS: %s", runtime.GOOS)
	}

	// Add common FFmpeg arguments
	commonArgs := []string{
		"-hide_banner", "-loglevel", "error",
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-vf",
	}

	// Compose the overlay filter if callsign is set
	var vfArg string
	if *callsign != "" {
		// Overlay callsign: bottom left, white text, black box background for legibility
		// You may want to customize fontfile path or font size as needed
		vfArg = fmt.Sprintf("scale=%d:%d,fps=30000/1001,drawbox=x=0:y=ih-40:w=iw:h=40:color=black@0.6:t=fill,drawtext=fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf:text='%s':x=10:y=h-35:fontcolor=white:fontsize=32:borderw=2:bordercolor=black", FrameWidth, FrameHeight, *callsign)
	} else {
		vfArg = fmt.Sprintf("scale=%d:%d,fps=30000/1001", FrameWidth, FrameHeight)
	}
	commonArgs = append(commonArgs, vfArg, "-")
	ffmpegArgs = append(ffmpegArgs, commonArgs...)

	ffmpegCmd := exec.Command("ffmpeg", ffmpegArgs...)

	// --- Start FFmpeg and the rest of the application ---
	ffmpegStdout, err := ffmpegCmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get FFmpeg stdout pipe: %v", err)
	}
	if err := ffmpegCmd.Start(); err != nil {
		log.Fatalf("Failed to start FFmpeg: %v", err)
	}
	log.Println("FFmpeg process started to capture webcam...")

	// --- HackRF setup ---
	if err := hackrf.Init(); err != nil {
		log.Fatalf("hackrf.Init() failed: %v", err)
	}
	defer hackrf.Exit()
	dev, err := hackrf.Open()
	if err != nil {
		log.Fatalf("hackrf.Open() failed: %v", err)
	}
	defer dev.Close()

	txFrequencyHz := uint64(*freq * 1_000_000)

	var outputFrequency float64
	if *rtl {
		outputFrequency = 2_400_000
	} else {
		outputFrequency = float64(*sampleRate)
	}

	// Set HackRF frequency and sample rate
	if err := dev.SetFreq(txFrequencyHz); err != nil {
		log.Fatalf("SetFreq failed: %v", err)
	}
	if err := dev.SetSampleRate(outputFrequency); err != nil {
		log.Fatalf("SetSampleRate failed: %v", err)
	}
	if err := dev.SetTXVGAGain(*gain); err != nil {
		log.Fatalf("SetTXVGAGain failed: %v", err)
	}
	if err := dev.SetAmpEnable(true); err != nil {
		log.Fatalf("SetAmpEnable failed: %v", err)
	}

	ntsc := NewNTSC(outputFrequency)

	// Goroutine to read video frames from FFmpeg
	go func() {
		for {
			_, err := io.ReadFull(ffmpegStdout, ntsc.rawFrameBuffer)
			if err != nil {
				if err == io.EOF {
					log.Println("FFmpeg stream ended.")
				} else {
					log.Printf("Error reading from FFmpeg: %v", err)
				}
				break
			}
		}
	}()

	// NTSC Generator Goroutine
	go func() {
		frameDuration := time.Second / time.Duration(ntsc.frameRate)
		ticker := time.NewTicker(frameDuration)
		defer ticker.Stop()
		for range ticker.C {
			ntsc.ntscFrameMutex.Lock()
			ntsc.GenerateFullFrame()
			ntsc.ntscFrameMutex.Unlock()
		}
	}()

	log.Printf("Starting NTSC transmission on %.3f MHz at %.1f Hz sample rate...", float64(txFrequencyHz)/1e6, outputFrequency)

	// Start HackRF Transmission
	var sampleCounter int = 0
	err = dev.StartTX(func(buf []byte) error {
		samplesToWrite := len(buf) / 2
		ntsc.ntscFrameMutex.RLock()
		defer ntsc.ntscFrameMutex.RUnlock()
		for i := 0; i < samplesToWrite; i++ {
			ire := ntsc.ntscFrameBuffer[sampleCounter]
			amplitude := ntsc.ireToAmplitude(ire)
			i_sample := int8(amplitude * 127.0)
			q_sample := int8(0)
			buf[i*2] = byte(i_sample)
			buf[i*2+1] = byte(q_sample)
			sampleCounter++
			if sampleCounter >= len(ntsc.ntscFrameBuffer) {
				sampleCounter = 0
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("StartTX failed: %v", err)
	}
	log.Println("Transmission is live. Press Ctrl+C to stop.")
	ffmpegCmd.Wait()
}