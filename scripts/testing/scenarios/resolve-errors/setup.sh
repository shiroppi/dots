# Items git cannot hold, inside an auto source.
set -eu
ln -s does-not-exist special/broken
ln -s loop-b special/loop-a
ln -s loop-a special/loop-b
mkfifo special/fifo
