# Maintainer: David Dunn
# VCS package built from the local checkout so uncommitted work is included.
# For a reproducible release build, replace prepare() with:
#   source=("git+https://github.com/algorhythmic/heimdall.git")
# and drop the local-tree copy below.
pkgname=heimdall-git
pkgver=r25.gd3e4694
pkgrel=1
pkgdesc="Verified session and workspace continuity daemon with a terminal interface (Hyprland, Herdr, Chromium)"
arch=('x86_64' 'aarch64')
url="https://github.com/algorhythmic/heimdall"
license=('custom')
depends=()
makedepends=('go' 'git')
optdepends=(
  'herdr: terminal session bindings and workspace metadata'
  'neovim: editor task/checkpoint integration'
  'chromium: browser surface observation and pairing'
)
provides=('heimdall')
conflicts=('heimdall')
options=('!debug')
source=()
b2sums=()

pkgver() {
  cd "$startdir"
  printf "r%s.g%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)"
}

prepare() {
  # Working-tree content of every tracked + non-ignored file, so uncommitted
  # work is packaged while .tools/, bin/, demo-data/ and node_modules stay out.
  rm -rf "$srcdir/$pkgname"
  mkdir -p "$srcdir/$pkgname"
  cd "$startdir"
  git ls-files -co --exclude-standard -z | tar --null -T - -cf - | tar -C "$srcdir/$pkgname" -xf -
}

build() {
  cd "$srcdir/$pkgname"
  export CGO_ENABLED=0
  export GOFLAGS="-trimpath -mod=readonly"
  go build -o heimdall ./cmd/heimdall
}

check() {
  cd "$srcdir/$pkgname"
  go test ./...
}

package() {
  cd "$srcdir/$pkgname"
  install -Dm755 heimdall "$pkgdir/usr/bin/heimdall"

  # Unpacked browser extension; load from developer mode or reference from
  # `heimdall browser setup` (see docs/guides/BROWSER-SETUP.md).
  mkdir -p "$pkgdir/usr/lib/heimdall/extension"
  cp -a extension/. "$pkgdir/usr/lib/heimdall/extension/"
  rm -rf "$pkgdir/usr/lib/heimdall/extension/test"

  # Neovim module; activate with:
  #   vim.opt.runtimepath:prepend('/usr/share/heimdall/neovim')
  mkdir -p "$pkgdir/usr/share/heimdall/neovim"
  cp -a integrations/neovim/. "$pkgdir/usr/share/heimdall/neovim/"

  install -Dm644 README.md "$pkgdir/usr/share/doc/heimdall/README.md"
  cp -a docs "$pkgdir/usr/share/doc/heimdall/docs"
  install -Dm644 examples/neovim-heimdall.lua "$pkgdir/usr/share/doc/heimdall/examples/neovim-heimdall.lua"
  install -Dm644 examples/herdr-sidebar.toml "$pkgdir/usr/share/doc/heimdall/examples/herdr-sidebar.toml"
}
