import { frameByteLength } from "./render";
import type { FrameSize, VideoFrame } from "./types";
import { ffmpegFrameArgs, ffplayAudioArgs } from "./process";

type FrameHandler = (frame: VideoFrame) => void;

export class MediaSession {
  private frameProcess?: Bun.Subprocess;
  private audioProcess?: Bun.Subprocess;
  private stopped = false;

  constructor(
    private readonly inputURL: string,
    private readonly ffmpegPath: string,
    private readonly ffplayPath: string,
  ) {}

  startVideo(size: FrameSize, seekSeconds: number, onFrame: FrameHandler) {
    this.stopVideo();
    this.stopped = false;

    const process = Bun.spawn([this.ffmpegPath, ...ffmpegFrameArgs(this.inputURL, size, seekSeconds)], {
      stdout: "pipe",
      stderr: "pipe",
    });
    this.frameProcess = process;
    void this.readFrames(process, size, onFrame);
  }

  startAudio(seekSeconds: number, muted: boolean, volume: number) {
    this.stopAudio();
    if (muted) {
      return;
    }

    this.audioProcess = Bun.spawn([
      this.ffplayPath,
      ...ffplayAudioArgs(this.inputURL, seekSeconds, muted, volume),
    ], {
      stdout: "ignore",
      stderr: "ignore",
    });
  }

  stopVideo() {
    this.frameProcess?.kill();
    this.frameProcess = undefined;
  }

  stopAudio() {
    this.audioProcess?.kill();
    this.audioProcess = undefined;
  }

  stop() {
    this.stopped = true;
    this.stopVideo();
    this.stopAudio();
  }

  private async readFrames(process: Bun.Subprocess, size: FrameSize, onFrame: FrameHandler) {
    if (!process.stdout) {
      return;
    }

    const stdout = process.stdout;
    if (typeof stdout === "number") {
      return;
    }

    const reader = stdout.getReader();
    const bytesPerFrame = frameByteLength(size);
    let pending: Uint8Array<ArrayBufferLike> = new Uint8Array(0);

    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done || this.stopped) {
          return;
        }

        pending = appendBytes(pending, value);
        while (pending.length >= bytesPerFrame) {
          const frameBytes = pending.slice(0, bytesPerFrame);
          pending = pending.slice(bytesPerFrame);
          onFrame({ width: size.width, height: size.height, data: frameBytes });

          // Keep only fresh frames when the terminal render loop is slower than decoding.
          if (pending.length > bytesPerFrame * 2) {
            pending = pending.slice(pending.length - bytesPerFrame);
          }
        }
      }
    } finally {
      reader.releaseLock();
    }
  }
}

function appendBytes(
  left: Uint8Array<ArrayBufferLike>,
  right: Uint8Array<ArrayBufferLike>,
): Uint8Array<ArrayBufferLike> {
  if (left.length === 0) {
    return right;
  }

  const merged = new Uint8Array(left.length + right.length);
  merged.set(left, 0);
  merged.set(right, left.length);

  return merged;
}
