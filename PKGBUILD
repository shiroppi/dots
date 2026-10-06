# Maintainer: shiroppi
#
# Local development build: packages dots from the source tree this PKGBUILD
# sits in (including uncommitted changes), not from a release tarball.
#   makepkg -si
# The binary is stamped "<version>-<githash>" (e.g. 0.1.0-223c2f0). pkgver
# cannot contain '-', so the package version uses '_' instead (0.1.0_223c2f0).
# <version> is the latest vX.Y.Z tag, or _basever when no tag exists yet.
# The AUR PKGBUILDs live in docs/aur/.

pkgname=dots
_basever=0.1.0
pkgver=0.1.0_223c2f0
pkgrel=1
pkgdesc='A brief, declarative, flexible dotfiles manager'
arch=('x86_64' 'aarch64' 'armv7h' 'i686' 'riscv64')
url='https://github.com/shiroppi/dots'
license=('MIT')
makedepends=('go' 'git')
provides=('dots')
conflicts=('dots-bin')

_version() {
  local base hash
  base=$(git -C "$startdir" describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)
  base=${base#v}
  hash=$(git -C "$startdir" rev-parse --short HEAD)
  printf '%s-%s' "${base:-$_basever}" "$hash"
}

pkgver() {
  _version | tr '-' '_'
}

build() {
  cd "$startdir"
  export CGO_CPPFLAGS="${CPPFLAGS}"
  export CGO_CFLAGS="${CFLAGS}"
  export CGO_CXXFLAGS="${CXXFLAGS}"
  export CGO_LDFLAGS="${LDFLAGS}"
  export GOPATH="${srcdir}/gopath"
  export GOFLAGS="-buildmode=pie -trimpath -mod=readonly -modcacherw"
  # -ldflags on the command line overrides any -ldflags in GOFLAGS, so
  # -linkmode=external is repeated here next to the version stamp.
  go build \
    -ldflags "-linkmode=external -extldflags \"${LDFLAGS}\" -X main.version=$(_version)" \
    -o "$srcdir/dots" ./cmd/dots
}

check() {
  cd "$startdir"
  export GOPATH="${srcdir}/gopath"
  export GOFLAGS="-mod=readonly -modcacherw"
  # Not ./...: makepkg's $startdir/pkg is unreadable, and go would walk into it.
  go test ./cmd/... ./internal/...
}

package() {
  install -Dm755 "$srcdir/dots" "$pkgdir/usr/bin/dots"
  install -Dm644 "$startdir/LICENSE" "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
  install -Dm644 "$startdir/README.md" "$pkgdir/usr/share/doc/$pkgname/README.md"
}
