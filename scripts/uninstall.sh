#!/usr/bin/env sh
set -eu

binary_name="argus"
install_dir="${XDG_BIN_HOME:-${HOME}/.local/bin}"

usage() {
	echo "usage: uninstall.sh"
	echo
	echo "Removes $install_dir/$binary_name."
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		*)
			echo "error: unknown option: $1" >&2
			usage >&2
			exit 1
			;;
	esac
	shift
done

if [ -e "$install_dir/$binary_name" ]; then
	rm -f "$install_dir/$binary_name"
	echo "removed $binary_name from $install_dir/$binary_name"
else
	echo "$binary_name was not found at $install_dir/$binary_name"
fi
