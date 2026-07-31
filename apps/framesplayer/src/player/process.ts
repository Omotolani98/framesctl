import type { FrameSize } from "./types";

export function ffmpegFrameArgs(inputURL: string, size: FrameSize, seekSeconds: number) {
  const args = [
    "-hide_banner",
    "-loglevel", "error",
  ];

  if (seekSeconds > 0) {
    args.push("-ss", String(seekSeconds));
  }

  args.push(
    "-i", inputURL,
    "-an",
    "-vf", `scale=${size.width}:${size.height}:force_original_aspect_ratio=decrease,pad=${size.width}:${size.height}:(ow-iw)/2:(oh-ih)/2`,
    "-r", "12",
    "-f", "rawvideo",
    "-pix_fmt", "rgb24",
    "pipe:1",
  );

  return args;
}

export function ffplayAudioArgs(inputURL: string, seekSeconds: number, muted: boolean, volume: number) {
  const args = ["-hide_banner", "-loglevel", "error", "-nodisp", "-autoexit"];

  if (seekSeconds > 0) {
    args.push("-ss", String(seekSeconds));
  }

  args.push("-volume", muted ? "0" : String(Math.max(0, Math.min(100, volume))), inputURL);

  return args;
}
