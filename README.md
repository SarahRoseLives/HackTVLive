# HackTVLive

HackTVLive is a Go application that captures live video from your webcam and transmits it as an NTSC analog television signal using a HackRF SDR device. It leverages FFmpeg for video capture and processing, and the [`go-hackrf`](https://github.com/samuel/go-hackrf) library for interfacing with HackRF hardware.

## Features

- **Live Webcam Capture**: Uses FFmpeg to grab video from your webcam and scale it to NTSC resolution.
- **NTSC Signal Generation**: Converts RGB video frames into NTSC color video with proper sync, blanking, and color burst.
- **SDR Transmission**: Transmits the NTSC signal over the air using a HackRF device at your specified frequency.
- **Cross-Platform**: Works on Linux, macOS, and Windows (with platform-specific FFmpeg input options).
- **Callsign Overlay**: Optionally overlays your callsign on the video for identification.


### Options

- `-freq` (required): Transmit frequency in MHz (e.g., 427.25)
- `-samplerate`: HackRF sample rate in Hz (default: 8000000)
- `-gain`: TX VGA gain (0-47, default: 47)
- `-device`: Video device name or index (platform-dependent, see below)
- `-callsign`: Callsign to overlay (optional)


## How It Works

1. **Video Capture**: FFmpeg grabs raw RGB video frames from your webcam and optionally overlays your callsign.
2. **NTSC Generation**: The Go code converts the video frames into NTSC signal format, including all sync pulses and color encoding.
3. **RF Transmission**: The NTSC signal is sent to the HackRF, which transmits it at the specified frequency.

## Safety & Legal Notice

**Transmitting on TV frequencies may be illegal in your country without a license.**
HackTVLive is intended for educational and experimental use only. Always operate within your local laws and regulations.