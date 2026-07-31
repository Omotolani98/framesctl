export function HelpDialog({ open }: { open: boolean }) {
  if (!open) {
    return null;
  }

  return (
    <box
      position="absolute"
      left={4}
      top={3}
      width={54}
      border
      borderStyle="rounded"
      padding={1}
      flexDirection="column"
      backgroundColor="#18181b"
    >
      <text fg="#f4f4f5">framesplayer keys</text>
      <text fg="#a1a1aa">space play/pause · ←/→ seek 5s · shift+←/→ seek 30s</text>
      <text fg="#a1a1aa">↑/↓ volume · m mute · ? help · q quit</text>
    </box>
  );
}
