# framesctl

Upload videos, transcode them to HLS, and play public share links from the terminal.

## Install macOS ARM64 binaries

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  https://raw.githubusercontent.com/Omotolani98/framesctl/main/install.sh | sh
```

Install a specific release:

```sh
curl --proto '=https' --tlsv1.2 -fsSL \
  https://raw.githubusercontent.com/Omotolani98/framesctl/main/install.sh | \
  VERSION=v0.1.0 sh
```

The installer downloads signed-by-release checksummed assets from GitHub Releases and installs:

- `framesctl`
- `framesplayer`

Terminal playback requires `ffmpeg`; audio playback also requires `ffplay`.

## Configure the CLI

The first CLI run creates `~/.framesctl/config.yaml`. Point it at your backend:

```yaml
api:
  base_url: https://framesrvr.tolaniverse.xyz
```

## Play a share

```sh
framesctl play https://framesrvr.tolaniverse.xyz/watch/<token>
```

## Release policy

Only SemVer tags on commits that are already on `main` publish releases.

```sh
git checkout main
git pull
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```
