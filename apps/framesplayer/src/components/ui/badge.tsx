type BadgeIntent = "neutral" | "success" | "warning" | "danger";

const colors: Record<BadgeIntent, { fg: string; bg: string }> = {
  neutral: { fg: "#d4d4d8", bg: "#27272a" },
  success: { fg: "#052e16", bg: "#22c55e" },
  warning: { fg: "#422006", bg: "#f59e0b" },
  danger: { fg: "#450a0a", bg: "#ef4444" },
};

export function Badge({ label, intent = "neutral" }: { label: string; intent?: BadgeIntent }) {
  const color = colors[intent];

  return (
    <box backgroundColor={color.bg} paddingLeft={1} paddingRight={1}>
      <text fg={color.fg}>{label}</text>
    </box>
  );
}
