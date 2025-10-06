package decoder

import (
	"log"
	"math"
	"sync"
	"rtltv/config" // Import our config package
)

// VSyncState defines the states for the vertical sync detection state machine.
type VSyncState int

const (
	// StateSearchVSync is the default state, looking for a V-sync sequence to start.
	StateSearchVSync VSyncState = iota
	// StateInVSync is active when the decoder has detected one or more V-sync
	// serration pulses and is expecting more to follow.
	StateInVSync
)

// Decoder processes I/Q samples into video frames.
type Decoder struct {
	frameBuffer   []byte
	displayBuffer []byte
	frameMutex    sync.Mutex

	// Decoder state
	x, y                 int
	pixelCounter         int
	smoothedMax          float64
	smoothedMin          float64
	hSyncPulseWidth      int
	syncSearchWindow     int
	lineStartActiveVideo int
	lineEndActiveVideo   int

	// --- Fields for advanced sync ---
	sampleRate            float64    // SDR sample rate, needed for PLL resets
	initialSamplesPerLine float64    // The ideal number of samples per line, for reference
	samplesPerLine        float64    // The PLL's current estimate of samples per line
	hSyncErrorAccumulator float64    // The integrated error for the H-sync PLL (the "I" in PI)
	vSyncState            VSyncState // Current state of the V-sync state machine
	vSyncSerrationCounter int        // Counts consecutive V-sync serration pulses
}

// New creates and initializes a new Decoder.
func New(sampleRate float64) *Decoder {
	d := &Decoder{}
	d.sampleRate = sampleRate

	lineDuration := 1.0 / (config.FrameRate * float64(config.LinesPerFrame))
	d.initialSamplesPerLine = lineDuration * sampleRate
	d.samplesPerLine = d.initialSamplesPerLine // Start with the ideal value

	d.hSyncPulseWidth = int(config.HsyncDurationMicroseconds * 1e-6 * sampleRate)
	d.syncSearchWindow = int(d.samplesPerLine * 0.20) // Search in first 20% of line

	activeVideoStartUs := config.HsyncDurationMicroseconds + config.FrontPorchMicroseconds
	d.lineStartActiveVideo = int(activeVideoStartUs * 1e-6 * sampleRate)
	d.lineEndActiveVideo = d.lineStartActiveVideo + int(config.ActiveVideoMicroseconds*1e-6*sampleRate)

	d.frameBuffer = make([]byte, config.FrameWidth*config.FrameHeight*3)
	d.displayBuffer = make([]byte, config.FrameWidth*config.FrameHeight*3)

	d.smoothedMax = 128.0 // Initial AGC values
	d.smoothedMin = 0.0

	// Initialize sync state
	d.vSyncState = StateSearchVSync
	d.hSyncErrorAccumulator = 0.0

	log.Printf("Decoder initialized: %.1f samples/line, hSync width ~%d samples", d.samplesPerLine, d.hSyncPulseWidth)
	log.Printf("Active Video: from sample %d to %d", d.lineStartActiveVideo, d.lineEndActiveVideo)

	return d
}

// ProcessIQ demodulates and decodes a chunk of I/Q data.
func (d *Decoder) ProcessIQ(iq []byte) {
	// AM Demodulation & AGC update
	amSignal := make([]float64, len(iq)/2)
	localMax, localMin := 0.0, 255.0
	for i := range amSignal {
		iqI := float64(int(iq[i*2]) - 127)
		iqQ := float64(int(iq[i*2+1]) - 127)
		mag := math.Sqrt(iqI*iqI + iqQ*iqQ)
		amSignal[i] = mag
		if mag > localMax {
			localMax = mag
		}
		if mag < localMin {
			localMin = mag
		}
	}
	d.smoothedMax = d.smoothedMax*0.95 + localMax*0.05
	d.smoothedMin = d.smoothedMin*0.95 + localMin*0.05

	// Define signal levels based on smoothed AGC
	syncTipLevel := d.smoothedMax
	peakWhiteLevel := d.smoothedMin
	syncThreshold := syncTipLevel * 0.75
	blackLevel := syncTipLevel * 0.65
	levelCoeff := 255.0 / (blackLevel - peakWhiteLevel + 1e-6)

	for _, mag := range amSignal {
		// --- Sync Detection ---
		if d.x < d.syncSearchWindow {
			if mag >= syncThreshold {
				d.pixelCounter++
			} else {
				if d.pixelCounter > d.hSyncPulseWidth/2 { // Found a pulse

					// --- V-Sync State Machine & H-Sync PLL ---
					isLongPulse := d.pixelCounter > d.hSyncPulseWidth*2

					switch d.vSyncState {
					case StateSearchVSync:
						// Look for the start of a V-sync sequence near the frame end
						if d.y > (config.FrameHeight-20) && isLongPulse {
							d.vSyncState = StateInVSync
							d.vSyncSerrationCounter = 1
						} else {
							// --- H-Sync PLL Logic ---
							// 1. Calculate error: how far was the pulse from where we expected it?
							error := float64(d.x) - d.samplesPerLine

							// 2. PI Controller: adjust our line length estimate
							// Reduced gains for stability
							const Kp = 0.002 // Proportional gain: immediate reaction to the error
							const Ki = 0.0001 // Integral gain: corrects for long-term drift
							d.hSyncErrorAccumulator += error * Ki
							correction := (error * Kp) + d.hSyncErrorAccumulator

							// *** THIS IS THE FIX: Change from -= to += ***
							// If pulse is late (error > 0), we need to INCREASE our line length estimate.
							d.samplesPerLine += correction

							// 3. Clamp the adjustment to prevent wild swings from noise
							if d.samplesPerLine < d.initialSamplesPerLine*0.95 {
								d.samplesPerLine = d.initialSamplesPerLine * 0.95
							}
							if d.samplesPerLine > d.initialSamplesPerLine*1.05 {
								d.samplesPerLine = d.initialSamplesPerLine * 1.05
							}

							// 4. Advance to next line
							d.y++
							d.x = 0
						}

					case StateInVSync:
						if isLongPulse && d.vSyncSerrationCounter < 6 {
							d.vSyncSerrationCounter++ // It's another pulse in the sequence
						} else {
							// The sequence ended. Check if it was a valid V-sync.
							if d.vSyncSerrationCounter >= 3 {
								// *** V-SYNC CONFIRMED ***
								d.y = 0
								d.x = 0
								// Reset the H-sync PLL to its ideal state
								d.samplesPerLine = d.initialSamplesPerLine
								d.hSyncErrorAccumulator = 0.0
							}
							// If not, it was a false alarm. The next pulse will be handled as H-sync.
							d.vSyncState = StateSearchVSync
							d.vSyncSerrationCounter = 0
						}
					}

					d.pixelCounter = 0
					continue // CRUCIAL: Skip to next sample after handling sync
				}
				d.pixelCounter = 0
			}
		}

		// --- Video Drawing ---
		if d.y >= 0 && d.y < config.FrameHeight && d.x >= d.lineStartActiveVideo && d.x < d.lineEndActiveVideo {
			samplesInActiveVideo := float64(d.lineEndActiveVideo - d.lineStartActiveVideo)
			relativeSample := float64(d.x - d.lineStartActiveVideo)
			pixelX := int(relativeSample / samplesInActiveVideo * float64(config.FrameWidth))

			if pixelX >= 0 && pixelX < config.FrameWidth {
				brightness := (blackLevel - mag) * levelCoeff
				if brightness < 0 {
					brightness = 0
				}
				if brightness > 255 {
					brightness = 255
				}
				pixelValue := byte(brightness)

				pixelIndex := (d.y*config.FrameWidth + pixelX) * 3
				d.frameBuffer[pixelIndex] = pixelValue
				d.frameBuffer[pixelIndex+1] = pixelValue
				d.frameBuffer[pixelIndex+2] = pixelValue
			}
		}

		d.x++

		// --- Flywheel & Frame Completion ---
		if d.x >= int(d.samplesPerLine) {
			d.x, d.y = 0, d.y+1 // Flywheel for coasting through complete signal loss
		}
		if d.y >= config.FrameHeight {
			d.y = 0
			d.frameMutex.Lock()
			copy(d.displayBuffer, d.frameBuffer)
			d.frameMutex.Unlock()
		}
	}
}

// GetDisplayFrame returns a thread-safe copy of the latest completed frame.
func (d *Decoder) GetDisplayFrame() []byte {
	d.frameMutex.Lock()
	defer d.frameMutex.Unlock()
	frameCopy := make([]byte, len(d.displayBuffer))
	copy(frameCopy, d.displayBuffer)
	return frameCopy
}