#!/bin/sh
set -eu

repo="${FRAMESCTL_REPO:-Omotolani98/framesctl}"
version="${VERSION:-}"
install_dir="${INSTALL_DIR:-$HOME/.local/bin}"
os_name="$(uname -s)"
arch_name="$(uname -m)"

case "$os_name" in
  Darwin) platform="darwin" ;;
  *)
    echo "unsupported OS: $os_name; this installer currently supports macOS ARM64 only" >&2
    exit 1
    ;;
esac

case "$arch_name" in
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "unsupported architecture: $arch_name; this installer currently supports macOS ARM64 only" >&2
    exit 1
    ;;
esac

if [ -z "$version" ]; then
  version="$(curl --proto '=https' --tlsv1.2 -fsSL "https://api.github.com/repos/${repo}/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
    awk 'NR == 1 { print; exit }')"
fi

if ! printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'; then
  echo "invalid VERSION: $version" >&2
  exit 1
fi

asset="framesctl_${version}_${platform}_${arch}.tar.gz"
base_url="https://github.com/${repo}/releases/download/${version}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

echo "Downloading ${asset}..."
curl --proto '=https' --tlsv1.2 -fsSL "${base_url}/${asset}" -o "${tmp_dir}/${asset}"
curl --proto '=https' --tlsv1.2 -fsSL "${base_url}/SHA256SUMS" -o "${tmp_dir}/SHA256SUMS"

if ! grep "  ${asset}\$" "${tmp_dir}/SHA256SUMS" > "${tmp_dir}/${asset}.sha256"; then
  echo "checksum entry for ${asset} not found" >&2
  exit 1
fi

(
  cd "$tmp_dir"
  shasum -a 256 -c "${asset}.sha256"
)

tar -tzf "${tmp_dir}/${asset}" > "${tmp_dir}/files"
while IFS= read -r file; do
  case "$file" in
    framesctl|framesplayer|README.md) ;;
    *)
      echo "archive contains unexpected path: $file" >&2
      exit 1
      ;;
  esac
done < "${tmp_dir}/files"

tar -xzf "${tmp_dir}/${asset}" -C "$tmp_dir"
mkdir -p "$install_dir"

install -m 0755 "${tmp_dir}/framesctl" "${install_dir}/framesctl.tmp"
install -m 0755 "${tmp_dir}/framesplayer" "${install_dir}/framesplayer.tmp"
mv "${install_dir}/framesctl.tmp" "${install_dir}/framesctl"
mv "${install_dir}/framesplayer.tmp" "${install_dir}/framesplayer"

echo "Installed framesctl and framesplayer to ${install_dir}"

case ":$PATH:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to PATH to run framesctl from anywhere." ;;
esac

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "Warning: ffmpeg is required for terminal video playback."
fi

if ! command -v ffplay >/dev/null 2>&1; then
  echo "Warning: ffplay is required for terminal audio playback."
fi

"${install_dir}/framesctl" --version >/dev/null
echo "framesctl $version is ready."
