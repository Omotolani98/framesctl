#!/bin/sh
set -eu

version="${1:?version is required}"
out_dir="${2:-dist/release}"
os_name="darwin"
arch="arm64"
root="$(git rev-parse --show-toplevel)"
archive_name="framesctl_${version}_${os_name}_${arch}.tar.gz"
package_dir="${out_dir}/package/framesctl_${version}_${os_name}_${arch}"

cd "$root"
rm -rf "$package_dir"
mkdir -p "$package_dir" "$out_dir"

go build \
  -trimpath \
  -ldflags="-s -w -X github.com/Omotolani98/framesctl/internals/version.Version=${version}" \
  -o "${package_dir}/framesctl" \
  ./cmd/framesctl

(
  cd apps/framesplayer
  bun install --frozen-lockfile
  bun run build:macos-arm64
)

cp apps/framesplayer/dist/framesplayer-darwin-arm64 "${package_dir}/framesplayer"
cp README.md "${package_dir}/README.md"
chmod 0755 "${package_dir}/framesctl" "${package_dir}/framesplayer"

tar -C "$package_dir" -czf "${out_dir}/${archive_name}" framesctl framesplayer README.md
echo "${out_dir}/${archive_name}"
