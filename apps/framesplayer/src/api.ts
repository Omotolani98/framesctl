import type { PlaybackMetadata } from "./player/types";

const defaultAPIBaseURL = "http://localhost:8080";

export function parseArgs(argv: string[]) {
  const options = {
    input: "",
    apiBaseURL: undefined as string | undefined,
    ffmpegPath: process.env.FFMPEG_PATH || "ffmpeg",
    ffplayPath: process.env.FFPLAY_PATH || "ffplay",
    audio: true,
  };

  for (let index = 0; index < argv.length; index++) {
    const value = argv[index];
    switch (value) {
      case "--api-base-url":
        options.apiBaseURL = argv[++index];
        break;
      case "--ffmpeg":
        options.ffmpegPath = argv[++index] || options.ffmpegPath;
        break;
      case "--ffplay":
        options.ffplayPath = argv[++index] || options.ffplayPath;
        break;
      case "--no-audio":
        options.audio = false;
        break;
      default:
        if (!options.input) {
          options.input = value || "";
        }
    }
  }

  return options;
}

export function resolveShare(input: string, apiBaseURL?: string) {
  const trimmed = input.trim();
  if (!trimmed) {
    throw new Error("share URL or token is required");
  }

  try {
    const url = new URL(trimmed);
    const parts = url.pathname.split("/").filter(Boolean);
    const token = parts.at(-1);
    if (!token) {
      throw new Error("share token is missing from URL");
    }

    return {
      token,
      apiBaseURL: apiBaseURL || url.origin,
    };
  } catch (error) {
    if (error instanceof TypeError) {
      return {
        token: trimmed,
        apiBaseURL: apiBaseURL || defaultAPIBaseURL,
      };
    }

    throw error;
  }
}

export async function fetchPlaybackMetadata(apiBaseURL: string, token: string): Promise<PlaybackMetadata> {
  const response = await fetch(`${apiBaseURL.replace(/\/$/, "")}/api/v1/public/shares/${encodeURIComponent(token)}`);
  if (response.status === 410) {
    throw new Error("share has expired");
  }
  if (!response.ok) {
    throw new Error(`playback API returned ${response.status}`);
  }

  return await response.json() as PlaybackMetadata;
}

export function absoluteURL(apiBaseURL: string, pathOrURL: string) {
  return new URL(pathOrURL, apiBaseURL.endsWith("/") ? apiBaseURL : `${apiBaseURL}/`).toString();
}
