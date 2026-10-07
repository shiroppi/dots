set -eu
ln -s nowhere broken-link
mkfifo fifo
echo 'I am a file' > "$HOME/.blocker"
mkdir -p "$HOME/real-dir"
ln -s real-dir "$HOME/.linkdir"
mkfifo "$HOME/.fifo-target"
