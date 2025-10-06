package video

import (
	"math"
	"sync"
)

// PAL struct holds all constants and state for generating the PAL signal.
type PAL struct {
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
	palFrameBuffer     []float64
	palFrameMutex      sync.RWMutex
}

// NewPAL creates a new PAL standard object.
func NewPAL(sampleRate float64) *PAL {
	p := &PAL{
		sampleRate:       sampleRate,
		frameRate:        25.0,
		linesPerFrame:    625,
		activeVideoLines: 576,
		fsc:              4433618.75,
		levelSync:        -40.0,
		levelBlanking:    0.0,
		levelBlack:       0.0,
		levelWhite:       100.0,
		burstAmplitude:   20.0,
	}
	lineDuration := 1.0 / (p.frameRate * float64(p.linesPerFrame))
	p.lineSamples = int(lineDuration * p.sampleRate)
	p.hSyncSamples = int(4.7e-6 * p.sampleRate)
	p.vSyncPulseSamples = int(27.3e-6 * p.sampleRate)
	p.eqPulseSamples = int(2.35e-6 * p.sampleRate)
	p.burstStartSamples = int(5.6e-6 * p.sampleRate)
	p.burstEndSamples = p.burstStartSamples + int(2.25e-6*p.sampleRate)
	p.activeStartSamples = int(10.5e-6 * p.sampleRate)
	p.activeSamples = int(52.0e-6 * p.sampleRate)
	p.rawFrameBuffer = make([]byte, FrameWidth*FrameHeight*3)
	p.palFrameBuffer = make([]float64, p.lineSamples*p.linesPerFrame)
	return p
}

// GenerateFullFrame creates a complete PAL frame from the raw pixel data.
func (p *PAL) GenerateFullFrame() {
	var subcarrierPhase float64 = 0.0
	phaseIncrement := 2.0 * math.Pi * p.fsc / p.sampleRate
	vToggle := 1.0

	for line := 1; line <= p.linesPerFrame; line++ {
		lineBuffer := p.generateLumaLine(line)
		isVBI := (line >= 624 || line <= 23) || (line >= 311 && line <= 336)

		if !isVBI {
			p.rawFrameMutex.RLock()
			for s := 0; s < p.lineSamples; s++ {
				burstPhaseOffset := 135.0 * (math.Pi / 180.0)
				if line%2 == 0 {
					burstPhaseOffset = -135.0 * (math.Pi / 180.0)
				}

				if s >= p.burstStartSamples && s < p.burstEndSamples {
					lineBuffer[s] += p.burstAmplitude * math.Sin(subcarrierPhase+burstPhaseOffset)
				} else if s >= p.activeStartSamples && s < (p.activeStartSamples+p.activeSamples) {
					_, u, v := p.getPixelYUV(line, s)
					lineBuffer[s] += u*math.Sin(subcarrierPhase) + (v*vToggle)*math.Cos(subcarrierPhase)
				}
				subcarrierPhase += phaseIncrement
			}
			p.rawFrameMutex.RUnlock()
		} else {
			subcarrierPhase += phaseIncrement * float64(p.lineSamples)
		}

		offset := (line - 1) * p.lineSamples
		copy(p.palFrameBuffer[offset:], lineBuffer)
		vToggle *= -1.0
	}
}

func (p *PAL) getPixelYUV(currentLine, sampleInLine int) (y, u, v float64) {
	var videoLine int
	if currentLine >= 24 && currentLine <= 310 {
		videoLine = currentLine - 24
	} else if currentLine >= 337 && currentLine <= 623 {
		videoLine = currentLine - 337 + p.activeVideoLines/2
	} else {
		return p.levelBlack, 0, 0
	}

	sampleInActiveVideo := sampleInLine - p.activeStartSamples
	pixelX := int(float64(sampleInActiveVideo) / float64(p.activeSamples) * FrameWidth)
	if videoLine < 0 || videoLine >= FrameHeight || pixelX < 0 || pixelX >= FrameWidth {
		return p.levelBlack, 0, 0
	}

	pixelIndex := (videoLine*FrameWidth + pixelX) * 3
	r := float64(p.rawFrameBuffer[pixelIndex])
	g := float64(p.rawFrameBuffer[pixelIndex+1])
	b := float64(p.rawFrameBuffer[pixelIndex+2])

	yVal := 0.299*r + 0.587*g + 0.114*b
	uVal := -0.147*r - 0.289*g + 0.436*b
	vVal := 0.615*r - 0.515*g - 0.100*b
	y = p.levelBlack + yVal/255.0*(p.levelWhite-p.levelBlack)
	u = uVal / 255.0 * (p.levelWhite - p.levelBlack) * 0.493
	v = vVal / 255.0 * (p.levelWhite - p.levelBlack) * 0.877
	return
}

func (p *PAL) generateLumaLine(currentLine int) []float64 {
	lineBuffer := make([]float64, p.lineSamples)
	for s := 0; s < p.lineSamples; s++ {
		lineBuffer[s] = p.levelBlanking
	}

	for s := 0; s < p.hSyncSamples; s++ {
		lineBuffer[s] = p.levelSync
	}

	if (currentLine >= 1 && currentLine <= 2) || (currentLine >= 313 && currentLine <= 314) {
		for s := p.lineSamples / 2; s < p.lineSamples/2+p.hSyncSamples; s++ {
			lineBuffer[s] = p.levelSync
		}
	}

	isVBI := (currentLine >= 624 || currentLine <= 23) || (currentLine >= 311 && currentLine <= 336)
	if !isVBI {
		p.rawFrameMutex.RLock()
		for s := 0; s < p.activeSamples; s++ {
			y, _, _ := p.getPixelYUV(currentLine, p.activeStartSamples+s)
			lineBuffer[p.activeStartSamples+s] = y
		}
		p.rawFrameMutex.RUnlock()
	}
	return lineBuffer
}

func (p *PAL) IreToAmplitude(ire float64) float64 {
	return ((ire - 100.0) / -140.0) * (1.0 - 0.125) + 0.125
}

func (p *PAL) FillTestPattern() {
	FillColorBars(p.rawFrameBuffer)
}

func (p *PAL) LockFrame()      { p.palFrameMutex.Lock() }
func (p *PAL) UnlockFrame()    { p.palFrameMutex.Unlock() }
func (p *PAL) RLockFrame()     { p.palFrameMutex.RLock() }
func (p *PAL) RUnlockFrame()   { p.palFrameMutex.RUnlock() }
func (p *PAL) LockRaw()        { p.rawFrameMutex.Lock() }
func (p *PAL) UnlockRaw()      { p.rawFrameMutex.Unlock() }
func (p *PAL) FrameBuffer() []float64 { return p.palFrameBuffer }
func (p *PAL) RawFrameBuffer() []byte { return p.rawFrameBuffer }