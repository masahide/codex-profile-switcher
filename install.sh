#!/usr/bin/env bash

set -Eeuo pipefail

repository="${CX_REPOSITORY:-masahide/codex-profile-switcher}"
requested_version="${1:-${CX_VERSION:-latest}}"

die() {
	printf 'cx installer: %s\n' "$*" >&2
	exit 1
}

need_command() {
	command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

download() {
	curl --fail --silent --show-error --location --retry 3 \
		--connect-timeout 10 --max-time 120 "$@"
}

if [[ "$#" -gt 1 ]]; then
	die "usage: install.sh [version]"
fi

need_command curl
need_command awk
need_command mktemp
need_command tar
need_command uname
need_command install
need_command mv

if [[ -n "${CX_INSTALL_DIR:-}" ]]; then
	install_dir="$CX_INSTALL_DIR"
elif [[ -n "${XDG_BIN_HOME:-}" ]]; then
	install_dir="$XDG_BIN_HOME"
else
	default_install_dir="${HOME:?HOME is not set}/.local/bin"
	install_dir="$default_install_dir"
fi
default_install_dir="${default_install_dir:-}"

if [[ "$install_dir" == *$'\n'* || "$install_dir" == *$'\r'* ]]; then
	die "CX_INSTALL_DIR must not contain a newline"
fi

case "$(uname -s)" in
Darwin)
	goos="darwin"
	;;
Linux)
	goos="linux"
	;;
*)
	die "unsupported operating system: $(uname -s)"
	;;
esac

case "$(uname -m)" in
x86_64|amd64)
	goarch="amd64"
	;;
aarch64|arm64)
	goarch="arm64"
	;;
*)
	die "unsupported CPU architecture: $(uname -m)"
	;;
esac

version="$requested_version"
if [[ "$version" == "latest" ]]; then
	api_url="${CX_RELEASE_API_URL:-https://api.github.com/repos/${repository}/releases/latest}"
	release_json="$(download \
		-H 'Accept: application/vnd.github+json' \
		-H 'X-GitHub-Api-Version: 2022-11-28' \
		-H 'User-Agent: cx-installer' \
		"$api_url")"
	version="$(printf '%s\n' "$release_json" | awk '
		{
			marker = "\"tag_name\""
			position = index($0, marker)
			if (position == 0) {
				next
			}
			value = substr($0, position + length(marker))
			sub(/^[[:space:]]*:[[:space:]]*"/, "", value)
			sub(/".*$/, "", value)
			print value
			exit
		}'
	)"
elif [[ "$version" != v* ]]; then
	version="v${version}"
fi

[[ "$version" =~ ^v[0-9][0-9A-Za-z._-]*$ ]] || die "invalid release version: $version"

if [[ -n "${CX_RELEASE_DOWNLOAD_BASE_URL:-}" ]]; then
	download_base="${CX_RELEASE_DOWNLOAD_BASE_URL%/}"
else
	download_base="https://github.com/${repository}/releases/download/${version}"
fi

archive="cx_${version}_${goos}_${goarch}.tar.gz"
temporary_dir="$(mktemp -d "${TMPDIR:-/tmp}/cx-install.XXXXXXXX")"
staged_binary=""

cleanup() {
	if [[ -n "$staged_binary" && -e "$staged_binary" ]]; then
		rm -f "$staged_binary"
	fi
	rm -rf "$temporary_dir"
}
trap cleanup EXIT

archive_path="$temporary_dir/$archive"
checksums_path="$temporary_dir/checksums.txt"
extract_dir="$temporary_dir/extract"

download -o "$archive_path" "${download_base}/${archive}"
download -o "$checksums_path" "${download_base}/checksums.txt"

expected_hash="$(awk -v filename="$archive" \
	'$2 == filename || $2 == "*" filename { print $1; exit }' \
	"$checksums_path")"
[[ "$expected_hash" =~ ^[[:xdigit:]]{64}$ ]] || die "checksum not found for $archive"

if command -v sha256sum >/dev/null 2>&1; then
	actual_hash="$(sha256sum "$archive_path" | awk '{ print $1 }')"
elif command -v shasum >/dev/null 2>&1; then
	actual_hash="$(shasum -a 256 "$archive_path" | awk '{ print $1 }')"
elif command -v openssl >/dev/null 2>&1; then
	actual_hash="$(openssl dgst -sha256 "$archive_path" | awk '{ print $NF }')"
else
	die "required checksum command not found: sha256sum, shasum, or openssl"
fi

expected_hash="$(printf '%s' "$expected_hash" | tr '[:upper:]' '[:lower:]')"
actual_hash="$(printf '%s' "$actual_hash" | tr '[:upper:]' '[:lower:]')"
[[ "$actual_hash" == "$expected_hash" ]] || die "checksum verification failed for $archive"

mkdir -p "$extract_dir"
tar -xzf "$archive_path" -C "$extract_dir"
binary_path="$extract_dir/cx"
[[ -f "$binary_path" ]] || die "release archive does not contain cx"

mkdir -p "$install_dir"
staged_binary="$(mktemp "$install_dir/.cx.XXXXXXXX")"
install -m 0755 "$binary_path" "$staged_binary"
mv -f "$staged_binary" "$install_dir/cx"
staged_binary=""

path_message=""
if [[ "${CX_NO_PATH_UPDATE:-0}" != "1" && ":${PATH:-}:" != *":${install_dir}:"* ]]; then
	if [[ -n "$default_install_dir" && "$install_dir" == "$default_install_dir" ]]; then
		case "${SHELL##*/}" in
		zsh)
			profile_file="${ZDOTDIR:-$HOME}/.zshrc"
			;;
		bash)
			profile_file="$HOME/.bashrc"
			;;
		*)
			profile_file="$HOME/.profile"
			;;
		esac
		path_line='export PATH="$HOME/.local/bin:$PATH"'
		if [[ ! -f "$profile_file" ]] || ! grep -Fqx "$path_line" "$profile_file"; then
			if {
				printf '\n# Added by cx installer\n%s\n' "$path_line" >> "$profile_file"
			}; then
				path_message="PATH updated in $profile_file: $HOME/.local/bin"
			else
				path_message="PATH update skipped; add $HOME/.local/bin to PATH before using cx"
			fi
		fi
	else
			path_message="PATH update skipped; add $install_dir to PATH before using cx"
	fi
fi

printf 'Installed cx %s for %s/%s at %s/cx\n' \
	"$version" "$goos" "$goarch" "$install_dir"
if [[ -n "$path_message" ]]; then
	printf '%s\n' "$path_message"
fi
printf 'Open a new shell or source your shell profile, then run: cx --help\n'
