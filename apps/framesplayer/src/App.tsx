import { useEffect, useMemo, useRef, useState } from "react";
import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react";
import { absoluteURL, fetchPlaybackMetadata, resolveShare } from "./api";
import { HelpDialog } from "./components/HelpDialog";
import { Badge } from "./components/ui/badge";
import { PlayerToggle } from "./components/ui/player-toggle";
import { MediaSession } from "./player/decoder";
import { fitFrameSize } from "./player/render";
import type { PlaybackMetadata, PlayerOptions, PlayerStatus, VideoFrame } from "./player/types";
import "./player/VideoCanvas";

const pollIntervalMS = 2000;

export function App({ options }: { options: PlayerOptions }) {
  const renderer = useRenderer();
  const dimensions = useTerminalDimensions();
  const share = useMemo(() => resolveShare(options.input, options.apiBaseURL), [options.input, options.apiBaseURL]);
  const frameSize = useMemo(
    () => fitFrameSize(dimensions.width, dimensions.height),
    [dimensions.width, dimensions.height],
  );

  const [metadata, setMetadata] = useState<PlaybackMetadata>();
  const [status, setStatus] = useState<PlayerStatus>("loading");
  const [frame, setFrame] = useState<VideoFrame>();
  const [playing, setPlaying] = useState(false);
  const [muted, setMuted] = useState(false);
  const [volume, setVolume] = useState(80);
  const [seekSeconds, setSeekSeconds] = useState(0);
  const [helpOpen, setHelpOpen] = useState(false);
  const [error, setError] = useState("");
  const sessionRef = useRef<MediaSession | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    let timer: Timer | undefined;

    const load = async () => {
      try {
        const next = await fetchPlaybackMetadata(share.apiBaseURL, share.token);
        if (cancelled) {
          return;
        }

        setMetadata(next);
        if (next.status === "ready" && next.master_url) {
          setStatus("ready");
          setPlaying(true);
          return;
        }

        if (next.status === "failed") {
          setStatus("failed");
          setError("transcoding failed");
          return;
        }

        setStatus("processing");
        timer = setTimeout(load, pollIntervalMS);
      } catch (err) {
        if (cancelled) {
          return;
        }

        const message = err instanceof Error ? err.message : String(err);
        setError(message);
        setStatus(message.includes("expired") ? "expired" : "failed");
      }
    };

    void load();

    return () => {
      cancelled = true;
      if (timer) {
        clearTimeout(timer);
      }
    };
  }, [share.apiBaseURL, share.token]);

  const playbackURL = metadata?.master_url ? absoluteURL(share.apiBaseURL, metadata.master_url) : "";

  useEffect(() => {
    sessionRef.current?.stop();
    sessionRef.current = undefined;

    if (!playbackURL || !playing) {
      setStatus(metadata?.status === "ready" ? "paused" : status);
      return;
    }

    const session = new MediaSession(playbackURL, options.ffmpegPath, options.ffplayPath);
    sessionRef.current = session;
    session.startVideo(frameSize, seekSeconds, setFrame);
    if (options.audio) {
      session.startAudio(seekSeconds, muted, volume);
    }
    setStatus("playing");

    return () => session.stop();
  }, [frameSize, metadata?.status, muted, options.audio, options.ffmpegPath, options.ffplayPath, playbackURL, playing, seekSeconds, status, volume]);

  useKeyboard((key) => {
    switch (key.name) {
      case "q":
      case "escape":
        sessionRef.current?.stop();
        renderer.destroy();
        break;
      case "space":
        setPlaying((value) => !value);
        break;
      case "left":
        setSeekSeconds((value) => Math.max(0, value - (key.shift ? 30 : 5)));
        setPlaying(true);
        break;
      case "right":
        setSeekSeconds((value) => value + (key.shift ? 30 : 5));
        setPlaying(true);
        break;
      case "up":
        setVolume((value) => Math.min(100, value + 5));
        break;
      case "down":
        setVolume((value) => Math.max(0, value - 5));
        break;
      case "m":
        setMuted((value) => !value);
        break;
      case "?":
        setHelpOpen((value) => !value);
        break;
    }
  });

  const title = metadata?.title || "framesplayer";

  return (
    <box width="100%" height="100%" flexDirection="column" backgroundColor="#09090b" padding={1} gap={1}>
      <box flexDirection="row" justifyContent="space-between" width="100%">
        <text fg="#f4f4f5">{title}</text>
        <Badge label={status.toUpperCase()} intent={statusIntent(status)} />
      </box>

      <box border borderStyle="rounded" width="100%" flexGrow={1} justifyContent="center" alignItems="center">
        {frame ? (
          <videoCanvas width={frameSize.width} height={Math.floor(frameSize.height / 2)} frame={frame} />
        ) : (
          <text fg="#a1a1aa">{error || (status === "processing" ? "transcoding…" : "waiting for frames…")}</text>
        )}
      </box>

      <box flexDirection="row" gap={1} alignItems="center">
        <PlayerToggle pressed={playing} label={playing ? "Pause" : "Play"} onPressedChange={setPlaying} />
        <PlayerToggle pressed={muted} label={muted ? "Muted" : `Vol ${volume}%`} onPressedChange={setMuted} />
        <text fg="#a1a1aa">seek {seekSeconds}s</text>
        <text fg="#71717a">space pause · ←/→ seek · m mute · ? help · q quit</text>
      </box>

      <HelpDialog open={helpOpen} />
    </box>
  );
}

function statusIntent(status: PlayerStatus) {
  switch (status) {
    case "playing":
    case "ready":
      return "success";
    case "processing":
    case "loading":
      return "warning";
    case "failed":
    case "expired":
      return "danger";
    default:
      return "neutral";
  }
}
