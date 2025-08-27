# HackTVLive

HackTVLive is a Go application that captures live video from your webcam and transmits it as an NTSC analog television signal using a HackRF SDR device. It leverages FFmpeg for video capture and processing, and the [`go-hackrf`](https://github.com/samuel/go-hackrf) library for interfacing with HackRF hardware.

## Features

- **Live Webcam Capture**: Uses FFmpeg to grab video from your webcam and scale it to NTSC resolution.
- **NTSC Signal Generation**: Converts RGB video frames into NTSC color video with proper sync, blanking, and color burst.
- **SDR Transmission**: Transmits the NTSC signal over the air using a HackRF device at your specified frequency.
- **Cross-Platform**: Works on Linux, macOS, and Windows (with platform-specific FFmpeg input options).
- **Callsign Overlay**: Optionally overlays your callsign on the video for identification.
- **Experimental Parameters**: Easily adjust transmission parameters for experimentation.

## Command-Line Flags

You can experiment with the following flags to customize your transmission:

- `-freq`: **Transmit frequency in MHz**  
  *Type:* `float`  
  *Default:* `427.25`  
  *Example:* `-freq 439.25`  
  *Description:* The center frequency for HackRF transmission. Common amateur TV frequencies include 427.25, 439.25, etc.

- `-bw`: **Channel bandwidth in MHz**  
  *Type:* `float`  
  *Default:* `8.0`  
  *Example:* `-bw 6`  
  *Description:* Sets both the sample rate and the channel width for NTSC. Standard NTSC channels are 6 MHz wide (use `-bw 6`). You may experiment with other values for signal shape and robustness.

- `-gain`: **TX VGA gain (0-47)**  
  *Type:* `int`  
  *Default:* `40`  
  *Example:* `-gain 47`  
  *Description:* Controls the HackRF transmit amplifier gain. Higher numbers mean more output power, but can cause distortion if set too high.

- `-device`: **Video device name or index**  
  *Type:* `string`  
  *Default:* `""` (auto-detects platform default)  
  *Example (Linux):* `-device /dev/video0`  
  *Example (macOS):* `-device 0`  
  *Example (Windows):* `-device "Integrated Webcam"`  
  *Description:* Selects the webcam to use. See below for how to list available devices.

- `-callsign`: **Callsign to overlay on the video**  
  *Type:* `string`  
  *Default:* `"NOCALL"`  
  *Example:* `-callsign N7XYZ`  
  *Description:* Overlays your callsign at the bottom left of the transmitted video for identification.

## Example Usage

Linux:
```sh
./HackTVLive -freq 427.25 -bw 6 -gain 40 -device /dev/video0 -callsign N0CALL
```

## Experimentation

HackTVLive is designed for experimentation:
- Try different `-freq` and `-bw` values to match local channel plans or test signal robustness.
- Adjust `-gain` for best power and minimum distortion.
- Change `-callsign` for identification or fun overlays.
- Use different video devices by specifying the `-device` flag.

If you omit `-device`, the application will try to use a reasonable default for your platform.

## How It Works

1. **Video Capture**: FFmpeg grabs raw RGB video frames from your webcam and optionally overlays your callsign.
2. **NTSC Generation**: The Go code converts the video frames into NTSC signal format, including all sync pulses and color encoding.
3. **RF Transmission**: The NTSC signal is sent to the HackRF, which transmits it at the specified frequency and bandwidth.

## Finding Your Video Device

To list available webcams:
- **Linux**: `v4l2-ctl --list-devices` or look in `/dev/video*`
- **macOS**: Devices are usually indexed (0, 1, etc.)
- **Windows**: Use the full device name as shown in device manager or FFmpeg logs.

## Safety & Legal Notice

**Transmitting on TV frequencies may be illegal in your country without a license.**
HackTVLive is intended for educational and experimental use only. Always operate within your local laws and regulations.

## Troubleshooting

- If the HackRF TX LED does not light up or the program exits immediately, check your device permissions and wiring.
- Stopping the application or transmission (Ctrl+C or "Stop Transmission" in GUI) will also turn off the HackRF TX LED.
- For best results, use a direct USB connection and avoid running other heavy processes while transmitting.

## Contributions

Pull requests and issues are welcome! See [`go-hackrf`](https://github.com/samuel/go-hackrf) for hardware support.

---