# Maintainer: shiroppi
#
# Local development build:
# Packages dots directly from the source tree containing this PKGBUILD.
# Includes uncommitted and untracked changes.
#
# Usage:
#   makepkg -si
#
# The package version is generated from the latest vX.Y.Z Git tag
# and the current commit hash.
#
# Examples:
#   0.1.0_223c2f0
#   0.1.0_223c2f0.dirty
#
# The .dirty suffix indicates that the working tree has changes.
#
# This PKGBUILD is intended for local development, not AUR distribution.
# The AUR PKGBUILDs are maintained separately in docs/aur/.

pkgname=dots
pkgver=0.1.0
pkgrel=1
pkgdesc='A brief, declarative, user-defined dotfiles manager'
arch=('x86_64' 'aarch64' 'armv7h' 'i686' 'riscv64')
url='https://github.com/shiroppi/dots'
license=('MIT')

makedepends=('go' 'git')
conflicts=('dots-bin')

_latest_tag() {
  git -C "$startdir" describe \
    --tags \
    --abbrev=0 \
    --match 'v[0-9]*' 2>/dev/null \
    | sed 's/^v//'
}

_basever=$(_latest_tag)
_basever=${_basever:-0.1.0}

_version() {
  local hash
  local dirty=''

  hash=$(git -C "$startdir" rev-parse --short HEAD) || return 1

  if [[ -n $(git -C "$startdir" status --porcelain --untracked-files=all) ]]; then
    dirty='.dirty'
  fi

  printf '%s-%s%s' "$_basever" "$hash" "$dirty"
}

# Arch Linux does not allow hyphens in pkgver.
pkgver() {
  _version | tr '-' '_'
}

# Build directly from the repository containing this PKGBUILD.
source=()
sha256sums=()

build() {
  cd "$startdir"

  go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${version}" \
    -o "$srcdir/dots" \
    .
}

package() {
  install -Dm755 "$srcdir/dots" \
    "$pkgdir/usr/bin/dots"

  install -Dm644 "$startdir/LICENSE" \
    "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
}
