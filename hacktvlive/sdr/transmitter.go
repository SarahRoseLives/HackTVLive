package sdr

import (
	"log"
	"sync"

	"github.com/samuel/go-hackrf/hackrf"
	"hacktvlive/config"
	"hacktvlive/video"
)

var debugLogOnce sync.Once

// Transmit configures an open HackRF device and starts the transmission stream.
func Transmit(dev *hackrf.Device, cfg *config.Config, v video.Standard) error {
	// NOTE: Init, Open, Close, and Exit are now handled in main.go

	txFrequencyHz := uint64(cfg.Frequency * 1_000_000)

	if err := dev.SetFreq(txFrequencyHz); err != nil {
		return err
	}
	if err := dev.SetSampleRate(cfg.SampleRate); err != nil {
		return err
	}
	if err := dev.SetTXVGAGain(cfg.Gain); err != nil {
		return err
	}
	if err := dev.SetAmpEnable(true); err != nil {
		return err
	}

	log.Printf("Starting transmission on %.3f MHz at %.1f Hz sample rate...",
		float64(txFrequencyHz)/1e6, cfg.SampleRate)

	var sampleCounter int = 0
	// StartTX is non-blocking and returns immediately.
	// The callback will run in the background until the device is closed by main.
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