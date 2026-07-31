import { expect, test } from "bun:test";
import { resolveShare } from "../api";
import { ffmpegFrameArgs, ffplayAudioArgs } from "./process";
import { fitFrameSize, frameByteLength } from "./render";

test("resolveShare accepts public watch URLs", () => {
  expect(resolveShare("https://framesrvr.run/watch/token-1")).toEqual({
    token: "token-1",
    apiBaseURL: "https://framesrvr.run",
  });
});

test("resolveShare accepts raw tokens", () => {
  expect(resolveShare("token-1", "https://api.example")).toEqual({
    token: "token-1",
    apiBaseURL: "https://api.example",
  });
});

test("ffmpegFrameArgs decodes bounded RGB frames", () => {
  const args = ffmpegFrameArgs("https://api.example/master.m3u8", { width: 80, height: 40 }, 12);
  expect(args).toContain("-ss");
  expect(args).toContain("rawvideo");
  expect(args).toContain("rgb24");
  expect(args.join(" ")).toContain("scale=80:40");
});

test("ffplayAudioArgs mutes by setting zero volume", () => {
  const args = ffplayAudioArgs("https://api.example/master.m3u8", 0, true, 80);
  expect(args).toContain("-nodisp");
  expect(args).toContain("0");
});

test("fitFrameSize leaves space for controls", () => {
  const size = fitFrameSize(100, 30);
  expect(size.width).toBe(96);
  expect(size.height).toBe(50);
  expect(frameByteLength(size)).toBe(96 * 50 * 3);
});
