# Audio Switchboard

**100% vibecoded by GPT 6.1 Terra.**

A small Linux desktop app for quickly muting and unmuting individual audio streams, recording apps, output devices and input devices. It uses `pactl` with a running PulseAudio-compatible audio server such as PipeWire with `pipewire-pulse`. Changes apply immediately.

## Requirements

- Linux desktop with GTK 3 and WebKitGTK 4.1
- A working PulseAudio-compatible server and the `pactl` command
- Go 1.24 or newer, a C compiler and `pkg-config` to build

## Build and run

```sh
go build -o audio-switchboard .
./audio-switchboard
```

Run as your normal desktop user. The app opens a small GTK window and lists active apps and devices. Select a switch to enable or mute its audio; recording streams appear only while active.

The embedded interface is served on a random loopback-only port behind a randomly generated URL token. It does not listen on an external network interface. It controls audio through `pactl` and does not require root privileges.

## Tests

```sh
go test -race ./...
go vet ./...
```

## License and disclaimer

[BSD Zero Clause License (0BSD)](LICENSE). Use, modify, and distribute freely, including commercially. The software is provided **as is**, without warranties; the license disclaims liability. Use at your own risk.
