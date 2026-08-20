#!/bin/sh
# Installs the latest SetFree release for macOS or Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/mindsdb/setfree/main/install.sh | sh
#
# Safe to re-run: it always overwrites the previous install with the
# current latest release. No sudo, no system-wide changes, no editing of
# shell startup files.
set -eu

REPO="mindsdb/setfree"
INSTALL_DIR="${SETFREE_INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="setfree"

say() { printf '%s\n' "$*"; }
err() { printf 'setfree: %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "'$1' is required but wasn't found on PATH."
}

detect_platform() {
	os="$(uname -s)"
	case "$os" in
	Darwin | Linux) : ;;
	*) die "unsupported OS: $os (SetFree supports macOS and Linux here — see install.ps1 for Windows)" ;;
	esac

	arch="$(uname -m)"
	case "$arch" in
	arm64 | aarch64) arch="arm64" ;;
	x86_64 | amd64) arch="x86_64" ;;
	*) die "unsupported architecture: $arch" ;;
	esac

	PLATFORM_OS="$os"
	PLATFORM_ARCH="$arch"
}

# latest_version resolves the newest release tag without calling the
# rate-limited GitHub API: /releases/latest 302s to /releases/tag/<version>,
# and we just read where it points.
latest_version() {
	url="https://github.com/$REPO/releases/latest"
	location="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$url")" ||
		die "couldn't reach GitHub to find the latest release."
	version="${location##*/}"
	[ -n "$version" ] || die "couldn't determine the latest SetFree version from $url"
	printf '%s\n' "$version"
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "need either 'sha256sum' or 'shasum' to verify the download."
	fi
}

main() {
	need curl
	need tar

	detect_platform
	version="$(latest_version)"
	say "Installing SetFree $version for ${PLATFORM_OS}/${PLATFORM_ARCH}..."

	asset="setfree_${PLATFORM_OS}_${PLATFORM_ARCH}.tar.gz"
	base_url="https://github.com/$REPO/releases/download/$version"

	workdir="$(mktemp -d)"
	trap 'rm -rf "$workdir"' EXIT

	curl -fsSL -o "$workdir/$asset" "$base_url/$asset" ||
		die "couldn't download $asset from release $version."

	if curl -fsSL -o "$workdir/checksums.txt" "$base_url/checksums.txt" 2>/dev/null; then
		expected="$(awk -v f="$asset" '$2 == f {print $1}' "$workdir/checksums.txt")"
		if [ -n "$expected" ]; then
			actual="$(sha256_of "$workdir/$asset")"
			[ "$expected" = "$actual" ] || die "checksum mismatch for $asset — expected $expected, got $actual. Aborting."
		else
			err "warning: $asset not listed in checksums.txt; skipping verification."
		fi
	else
		err "warning: checksums.txt not available for $version; skipping verification."
	fi

	tar -xzf "$workdir/$asset" -C "$workdir"
	[ -f "$workdir/$BIN_NAME" ] || die "$asset didn't contain the expected '$BIN_NAME' binary."

	mkdir -p "$INSTALL_DIR"
	install -m 755 "$workdir/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME" 2>/dev/null ||
		{ cp "$workdir/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME" && chmod 755 "$INSTALL_DIR/$BIN_NAME"; }

	say "✓ Installed $BIN_NAME to $INSTALL_DIR/$BIN_NAME"

	case ":$PATH:" in
	*":$INSTALL_DIR:"*)
		say ""
		say "Run 'setfree' to get started."
		;;
	*)
		say ""
		say "$INSTALL_DIR isn't on your PATH yet. Add it:"
		say ""
		say "  export PATH=\"$INSTALL_DIR:\$PATH\""
		say ""
		say "then run 'setfree' to get started."
		;;
	esac
}

main "$@"
