package video

import (
	"math"
	"sync"
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

// NewNTSC creates a new NTSC standard object.
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

// GenerateFullFrame creates a complete NTSC frame from the raw pixel data.
func (n *NTSC) GenerateFullFrame() {
	var subcarrierPhase float64 = 0.0
	phaseIncrement := 2.0 * math.Pi * n.fsc / n.sampleRate
	for line := 1; line <= n.linesPerFrame; line++ {
		lineBuffer := n.generateLumaLine(line)
		isVBI := (line >= 1 && line <= 21) || (line >= 264 && line <= 284)
		if !isVBI {
			for s := 0; s < n.lineSamples; s++ {
				if s >= n.burstStartSamples && s < n.burstEndSamples {
					lineBuffer[s] += n.burstAmplitude * math.Sin(subcarrierPhase+math.Pi)
				} else if s >= n.activeStartSamples && s < (n.activeStartSamples+n.activeSamples) {
					_, i, q := n.getPixelYIQ(line, s)
					lineBuffer[s] += i*math.Cos(subcarrierPhase) + q*math.Sin(subcarrierPhase)
				}
				subcarrierPhase += phaseIncrement
			}
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
	case lineInField >= 1 && lineInField <= 3, lineInField >= 7 && lineInField <= 9:
		for s := 0; s < n.eqPulseSamples; s++ {
			lineBuffer[s], lineBuffer[halfLine+s] = n.levelSync, n.levelSync
		}
		return lineBuffer
	case lineInField >= 4 && lineInField <= 6:
		for s := 0; s < n.vSyncPulseSamples; s++ {
			lineBuffer[s], lineBuffer[halfLine+s] = n.levelSync, n.levelSync
		}
		return lineBuffer
	}
	for s := 0; s < n.hSyncSamples; s++ {
		lineBuffer[s] = n.levelSync
	}
	if !isVBI {
		for s := 0; s < n.activeSamples; s++ {
			y, _, _ := n.getPixelYIQ(currentLine, n.activeStartSamples+s)
			lineBuffer[n.activeStartSamples+s] = y
		}
	}
	return lineBuffer
}

func (n *NTSC) IreToAmplitude(ire float64) float64 {
	return ((ire - 100.0) / -140.0) * (1.0 - 0.125) + 0.125
}

func (n *NTSC) FillTestPattern() {
	FillColorBars(n.rawFrameBuffer)
}

func (n *NTSC) LockFrame()      { n.ntscFrameMutex.Lock() }
func (n *NTSC) UnlockFrame()    { n.ntscFrameMutex.Unlock() }
func (n *NTSC) RLockFrame()     { n.ntscFrameMutex.RLock() }
func (n *NTSC) RUnlockFrame()   { n.ntscFrameMutex.RUnlock() }
func (n *NTSC) LockRaw()        { n.rawFrameMutex.Lock() }
func (n *NTSC) UnlockRaw()      { n.rawFrameMutex.Unlock() }
func (n *NTSC) FrameBuffer() []float64 { return n.ntscFrameBuffer }
func (n *NTSC) RawFrameBuffer() []byte { return n.rawFrameBuffer }