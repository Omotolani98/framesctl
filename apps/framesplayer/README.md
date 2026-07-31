# framesplayer

OpenTUI terminal player for framesrvr public HLS shares.

```bash
bun install
bun run start https://framesrvr.run/watch/<token>
```

Runtime dependencies:

- `ffmpeg` for video frame decoding
- `ffplay` for optional audio playback

Controls: `space` play/pause, arrows seek/volume, `m` mute, `?` help, `q` quit.
