import {
  FrameBufferRenderable,
  RGBA,
  type FrameBufferOptions,
  type RenderContext,
} from "@opentui/core";
import { extend } from "@opentui/react";
import { isCompleteFrame, pixelOffset } from "./render";
import type { VideoFrame } from "./types";

const BLACK = RGBA.fromHex("#000000");

export class VideoCanvasRenderable extends FrameBufferRenderable {
  private currentFrame?: VideoFrame;

  constructor(ctx: RenderContext, options: FrameBufferOptions & { frame?: VideoFrame }) {
    super(ctx, options);
    if (options.frame) {
      this.frame = options.frame;
    }
  }

  set frame(frame: VideoFrame | undefined) {
    this.currentFrame = frame;
    this.drawFrame();
    this.requestRender();
  }

  get frame() {
    return this.currentFrame;
  }

  private drawFrame() {
    const frame = this.currentFrame;
    this.frameBuffer.clear(BLACK);
    if (!frame || !isCompleteFrame(frame)) {
      return;
    }

    const rows = Math.floor(frame.height / 2);
    for (let y = 0; y < rows; y++) {
      const topY = y * 2;
      const bottomY = topY + 1;
      for (let x = 0; x < frame.width; x++) {
        const top = pixelOffset(frame, x, topY);
        const bottom = pixelOffset(frame, x, bottomY);
        this.frameBuffer.setCell(
          x,
          y,
          "▀",
          RGBA.fromInts(frame.data[top] ?? 0, frame.data[top + 1] ?? 0, frame.data[top + 2] ?? 0, 255),
          RGBA.fromInts(frame.data[bottom] ?? 0, frame.data[bottom + 1] ?? 0, frame.data[bottom + 2] ?? 0, 255),
        );
      }
    }
  }
}

declare module "@opentui/react" {
  interface OpenTUIComponents {
    videoCanvas: typeof VideoCanvasRenderable;
  }
}

extend({ videoCanvas: VideoCanvasRenderable });
