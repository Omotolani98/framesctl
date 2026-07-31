import { Toggle } from "@tuiparts/react/toggle";

export function PlayerToggle({
  pressed,
  label,
  onPressedChange,
}: {
  pressed: boolean;
  label: string;
  onPressedChange: (pressed: boolean) => void;
}) {
  return (
    <Toggle pressed={pressed} onPressedChange={(nextPressed) => onPressedChange(nextPressed)}>
      <box border paddingLeft={1} paddingRight={1} borderStyle="rounded">
        <text fg={pressed ? "#22c55e" : "#d4d4d8"}>{label}</text>
      </box>
    </Toggle>
  );
}
