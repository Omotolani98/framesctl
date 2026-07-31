#!/usr/bin/env bun
import { createCliRenderer } from "@opentui/core";
import { createRoot } from "@opentui/react";
import { App } from "./App";
import { parseArgs } from "./api";

const options = parseArgs(Bun.argv.slice(2));
if (!options.input) {
  console.error("usage: framesplayer <share-url-or-token> [--api-base-url <url>] [--no-audio]");
  process.exit(2);
}

const renderer = await createCliRenderer({ exitOnCtrlC: true });
createRoot(renderer).render(<App options={options} />);
