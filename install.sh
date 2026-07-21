#!/bin/sh
# Build pkm and install it to a directory on PATH.
#
# Usage:
#   ./install.sh              install to ~/.local/bin
#   ./install.sh /some/dir     install to /some/dir
#   PREFIX=/some/dir ./install.sh   same, via env var
#   ./install.sh --help
set -eu

show_help() {
	cat <<'EOF'
install.sh - build pkm and install it to a directory on PATH

Usage:
  ./install.sh [DIR]
  PREFIX=DIR ./install.sh

Options:
  DIR / PREFIX   Install directory (default: ~/.local/bin)
  -h, --help     Show this help and exit

If the install directory isn't already on $PATH, an export line is
appended to your shell rc file ($HOME/.zshrc or $HOME/.bashrc, chosen
from $SHELL) so future shells pick it up. The line is never added twice,
and no existing rc file content is ever rewritten or reordered.
EOF
}

for arg in "$@"; do
	case "$arg" in
	-h | --help)
		show_help
		exit 0
		;;
	esac
done

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

prefix="${PREFIX:-}"
if [ $# -gt 0 ]; then
	prefix="$1"
fi
if [ -z "$prefix" ]; then
	prefix="$HOME/.local/bin"
fi

echo "Building pkm..."
if ! (cd "$script_dir" && go build -o pkm ./cmd/pkm); then
	echo "error: build failed, nothing installed" >&2
	exit 1
fi

mkdir -p "$prefix"
cp "$script_dir/pkm" "$prefix/pkm"
chmod +x "$prefix/pkm"
echo "Installed pkm to $prefix/pkm"

# Match PATH by whole component, not substring: a naive `case $PATH in
# *"$prefix"*)` would false-positive on "/home/x/.local/binary" for a
# "/home/x/.local/bin" prefix.
on_path=0
old_ifs=$IFS
IFS=:
for dir in $PATH; do
	if [ "$dir" = "$prefix" ]; then
		on_path=1
	fi
done
IFS=$old_ifs

if [ "$on_path" -eq 1 ]; then
	exit 0
fi

case "${SHELL:-}" in
*/zsh)
	rc_file="$HOME/.zshrc"
	;;
*/bash)
	rc_file="$HOME/.bashrc"
	;;
*)
	echo
	echo "$prefix is not on your PATH, and \$SHELL ($SHELL) isn't zsh or bash."
	echo "Add this line to your shell's startup file yourself:"
	echo "  export PATH=\"$prefix:\$PATH\""
	exit 0
	;;
esac

export_line="export PATH=\"$prefix:\$PATH\""

touch "$rc_file"
if grep -qF "$export_line" "$rc_file"; then
	exit 0
fi

printf '%s\n' "$export_line" >>"$rc_file"
echo
echo "Added to $rc_file:"
echo "  $export_line"
echo "Run 'source $rc_file' or open a new shell to pick it up."
