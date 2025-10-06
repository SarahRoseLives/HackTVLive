package sdr

import (
	"log"
	"math"
	"sync"

	"github.com/samuel/go-hackrf/hackrf"
	"hacktvlive/config"
	"hacktvlive/video"
)

// NewLowPassFilterTaps creates the coefficients (taps) for a FIR low-pass filter.
// A Blackman window is used for good performance.
func NewLowPassFilterTaps(numTaps int, bandwidth, sampleRate float64) []float64 {
	taps := make([]float64, numTaps)
	cutoffFreq := bandwidth / 2.0
	normalizedCutoff := cutoffFreq / sampleRate

	M := float64(numTaps - 1)
	var sum float64
	for i := 0; i < numTaps; i++ {
		n := float64(i)
		window := 0.42 - 0.5*math.Cos(2*math.Pi*n/M) + 0.08*math.Cos(4*math.Pi*n/M)

		var sinc float64
		if i == int(M/2) {
			sinc = 2 * math.Pi * normalizedCutoff
		} else {
			sinc = math.Sin(2*math.Pi*normalizedCutoff*(n-M/2)) / (n - M/2)
		}

		taps[i] = sinc * window
		sum += taps[i]
	}

	// Normalize the taps to have a gain of 1 at DC (0 Hz)
	for i := range taps {
		taps[i] /= sum
	}
	return taps
}

var debugLogOnce sync.Once

// Transmit configures an open HackRF device and starts the transmission stream.
func Transmit(dev *hackrf.Device, cfg *config.Config, v video.Standard) error {
	txFrequencyHz := uint64(cfg.Frequency * 1_000_000)

	if err := dev.SetFreq(txFrequencyHz); err != nil {
		return err
	}
	// Use the fixed sample rate from the config package
	if err := dev.SetSampleRate(config.FixedSampleRate); err != nil {
		return err
	}
	if err := dev.SetTXVGAGain(cfg.Gain); err != nil {
		return err
	}
	if err := dev.SetAmpEnable(false); err != nil {
		return err
	}

	log.Printf("Starting transmission on %.3f MHz with a %.2f MHz filter bandwidth (Sample Rate: %.1f Msps)...",
		float64(txFrequencyHz)/1e6, cfg.Bandwidth, config.FixedSampleRate/1e6)

	var sampleCounter int = 0
	// StartTX is non-blocking and returns immediately.
	// The callback is now simple again, only sending pre-filtered samples.
	return dev.StartTX(func(buf []byte) error {
		samplesToWrite := len(buf) / 2

		v.RLockFrame()
		defer v.RUnlockFrame()

		frameBuf := v.FrameBuffer()

		for i := 0; i < samplesToWrite; i++ {
			ire := frameBuf[sampleCounter]
			amplitude := v.IreToAmplitude(ire)

			iSample := int8(amplitude * 127.0)
			qSample := int8(0)

			buf[i*2] = byte(iSample)
			buf[i*2+1] = byte(qSample)

			sampleCounter++
			if sampleCounter >= len(frameBuf) {
				sampleCounter = 0
			}
		}
		return nil
	})
}