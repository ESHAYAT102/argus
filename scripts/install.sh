#!/usr/bin/env sh
set -eu

repo_url="https://github.com/ESHAYAT102/argus.git"
binary_name="argus"
install_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"
clone_dir=""

has() {
	command -v "$1" >/dev/null 2>&1
}

prompt_yes_no() {
	prompt="$1"

	if [ ! -r /dev/tty ] || [ ! -w /dev/tty ]; then
		echo "error: cannot prompt for confirmation without a terminal" >&2
		exit 1
	fi

	while true; do
		printf "%s [Y/n] " "$prompt" >/dev/tty
		IFS= read -r answer </dev/tty || answer=""
		case "$answer" in
			"" | [Yy] | [Yy][Ee][Ss])
				return 0
				;;
			[Nn] | [Nn][Oo])
				return 1
				;;
			*)
				echo "please answer Y or n" >/dev/tty
				;;
		esac
	done
}

cleanup() {
	if [ -n "${clone_dir:-}" ] && [ -d "$clone_dir" ]; then
		rm -rf "$clone_dir"
	fi
}

trap cleanup EXIT
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM

run_as_root() {
	if [ "$(id -u)" -eq 0 ]; then
		"$@"
	elif has sudo; then
		sudo "$@"
	else
		echo "error: sudo is required to install missing packages" >&2
		exit 1
	fi
}

install_dev_dependencies() {
	os="$(uname -s)"
	case "$os" in
		Darwin)
			if ! has brew; then
				echo "error: Homebrew is required to install missing packages on macOS" >&2
				echo "install Homebrew, then rerun this script" >&2
				exit 1
			fi
			brew install go git
			;;
		Linux)
			if has pacman; then
				run_as_root pacman -Sy --needed --noconfirm go git
			elif has apt-get; then
				run_as_root apt-get update
				run_as_root apt-get install -y golang-go git
			elif has dnf; then
				run_as_root dnf install -y golang git
			elif has yum; then
				run_as_root yum install -y golang git
			elif has zypper; then
				run_as_root zypper --non-interactive install go git
			elif has apk; then
				run_as_root apk add go git
			elif has xbps-install; then
				run_as_root xbps-install -Sy go git
			else
				echo "error: no supported package manager found to install Go and git" >&2
				echo "supported Linux package managers: pacman, apt-get, dnf, yum, zypper, apk, xbps-install" >&2
				exit 1
			fi
			;;
		*)
			echo "error: unsupported OS: $os" >&2
			exit 1
			;;
	esac
	hash -r 2>/dev/null || true
}

ensure_dev_dependencies() {
	if has go && has git; then
		return
	fi

	missing=""
	if ! has go; then
		missing="go"
	fi
	if ! has git; then
		if [ -n "$missing" ]; then
			missing="$missing and git"
		else
			missing="git"
		fi
	fi

	if ! prompt_yes_no "Install missing required software ($missing) now?"; then
		echo "error: $missing is required to install $binary_name" >&2
		exit 1
	fi

	install_dev_dependencies
	if ! has go; then
		echo "error: go is still unavailable after installation" >&2
		exit 1
	fi

	if ! has git; then
		echo "error: git is still unavailable after installation" >&2
		exit 1
	fi
}

ensure_dev_dependencies

mkdir -p "$install_dir"

clone_dir="$(mktemp -d "${TMPDIR:-/tmp}/argus.XXXXXX")"
echo "cloning $repo_url"
git clone --depth 1 "$repo_url" "$clone_dir"

echo "building $binary_name"
(
	cd "$clone_dir"
	go build -o "$install_dir/$binary_name" ./cmd/argus
)
chmod +x "$install_dir/$binary_name"

echo "installed $binary_name to $install_dir/$binary_name"
case ":$PATH:" in
	*":$install_dir:"*) ;;
	*)
		echo "warning: $install_dir is not in PATH"
		echo "add this to your shell profile:"
		echo "  export PATH=\"$install_dir:\$PATH\""
		;;
esac
