package sdr

import (
	"fmt"
	"log"
	"rtltv/config"
	rtl "github.com/jpoirier/gortlsdr"
)

// SetupDevice initializes and configures the RTL-SDR device.
// This version matches the library you provided.
func SetupDevice(cfg *config.SDRConfig) (*rtl.Context, error) {
	devCount := rtl.GetDeviceCount()
	if devCount == 0 {
		return nil, fmt.Errorf("no RTL-SDR devices found")
	}
	log.Printf("Found %d RTL-SDR device(s). Using device 0.", devCount)

	// Use the correct Open() function which returns a *Context
	dongle, err := rtl.Open(0)
	if err != nil {
		return nil, fmt.Errorf("error opening RTL-SDR device: %w", err)
	}

	// Configure device
	if err := dongle.SetCenterFreq(cfg.FrequencyHz); err != nil {
		dongle.Close()
		return nil, fmt.Errorf("SetCenterFreq failed: %w", err)
	}
	log.Printf("Tuned to frequency: %.3f MHz", float64(cfg.FrequencyHz)/1e6)

	if err := dongle.SetSampleRate(cfg.SampleRateHz); err != nil {
		dongle.Close()
		return nil, fmt.Errorf("SetSampleRate failed: %w", err)
	}
	log.Printf("Sample rate set to: %.3f MHz", float64(cfg.SampleRateHz)/1e6)

	if err := dongle.SetTunerGainMode(true); err != nil {
		dongle.Close()
		return nil, fmt.Errorf("SetTunerGainMode failed: %w", err)
	}
	if err := dongle.SetTunerGain(cfg.Gain); err != nil {
		dongle.Close()
		return nil, fmt.Errorf("SetTunerGain failed: %w", err)
	}
	log.Printf("Tuner gain set to MANUAL: %.1f dB", float64(cfg.Gain)/10.0)

	if err := dongle.ResetBuffer(); err != nil {
		dongle.Close()
		return nil, fmt.Errorf("ResetBuffer failed: %w", err)
	}

	return dongle, nil
}