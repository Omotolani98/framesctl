export type PlayerStatus = "idle" | "loading" | "processing" | "ready" | "playing" | "paused" | "failed" | "expired";

export interface PlaybackMetadata {
  title: string;
  status: string;
  expires_at: string | null;
  master_url?: string;
  poster_url?: string;
}

export interface PlayerOptions {
  input: string;
  apiBaseURL?: string;
  ffmpegPath: string;
  ffplayPath: string;
  audio: boolean;
}

export interface VideoFrame {
  width: number;
  height: number;
  data: Uint8Array<ArrayBufferLike>;
}

export interface FrameSize {
  width: number;
  height: number;
}
