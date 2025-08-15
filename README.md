# HackTVLive

HackTVLive is a Go-based utility that bridges **OBS Studio** streaming to an analog TV transmitter using [hacktv](https://github.com/fsphil/hacktv). It listens for an RTMP stream from OBS, optionally transcodes the input, and pipes it live to hacktv for analog transmission.

---

## Features

- **Dynamic scaling**: By default, output matches whatever resolution OBS feeds. You can override with environment variables.
- **Frequency selection**: Specify the transmission frequency in MHz via the `--freq` command line flag.
- **Audio control**: Enable or disable audio transmission with `--audio` (enabled by default).
- **Simple usage**: Provides helpful instructions if run without flags.

---

## Usage

### 1. OBS Studio Configuration

Add a custom RTMP output in OBS:

- **URL:** `rtmp://localhost:1935/live/stream`
- **Stream Key:** (leave blank or set as desired)

### 2. Running HackTVLive

If you run the program without any flags, you'll see usage instructions and the RTMP URL to add in OBS.

```sh
./HackTVLive
```

Example output:
```
No arguments provided.
To use this transmitter, add the following RTMP URL as a custom output in OBS Studio:
  rtmp://localhost:1935/live/stream
Then run this program with:
  --freq <frequency in MHz> [--audio=<true|false>]
Example:
  ./HackTVLive --freq 471.25 --audio=false
```

### 3. Start the transmitter

The minimum required flag is `--freq` (frequency in MHz):

```sh
./HackTVLive --freq 471.25
```

To disable audio transmission:

```sh
./HackTVLive --freq 471.25 --audio=false
```

### 4. Advanced: Environment Variables

You can override scaling, frame rate, and pixel format with environment variables:

- `SCALE_WIDTH` and `SCALE_HEIGHT` – Output resolution.
- `FPS` – Frames per second (default: 15).
- `PIX_FMT` – Pixel format (default: yuv420p).

Example:

```sh
SCALE_WIDTH=720 SCALE_HEIGHT=480 FPS=30 ./HackTVLive --freq 471.25
```

---

## Requirements

- [Go](https://golang.org/) (to build/run HackTVLive)
- [ffmpeg](https://ffmpeg.org/) (must be in your PATH)
- [hacktv](https://github.com/fsphil/hacktv) (must be built and available as `./hacktv`)

---

## How It Works

1. **Listens** for an RTMP stream from OBS.
2. **Transcodes** the video (if needed) using ffmpeg.
3. **Pipes** the output to hacktv for analog TV transmission at your specified frequency.