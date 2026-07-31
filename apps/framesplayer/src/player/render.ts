import type { FrameSize, VideoFrame } from "./types";

export function fitFrameSize(terminalWidth: number, terminalHeight: number): FrameSize {
  const chromeHeight = 5;
  const width = Math.max(20, terminalWidth - 4);
  const cellHeight = Math.max(6, terminalHeight - chromeHeight);

  return {
    width,
    height: cellHeight * 2,
  };
}

export function frameByteLength(size: FrameSize) {
  return size.width * size.height * 3;
}

export function isCompleteFrame(frame: VideoFrame) {
  return frame.data.length === frame.width * frame.height * 3;
}

export function pixelOffset(frame: VideoFrame, x: number, y: number) {
  return (y * frame.width + x) * 3;
}
