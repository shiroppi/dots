# Pre-existing items in the fake home that collide with the basic repository.
# Runs with HOME and REPO set; cwd is the repository.
set -eu
cd "$HOME"
mkdir -p .config .local/bin

# FileOrDir: a regular file.
echo '" my old vimrc (home)' > .vimrc
# FileOrDir: a real directory with nested content.
mkdir -p .config/tmux/plugins
echo '# my old tmux.conf (home)' > .config/tmux/tmux.conf
echo 'plugin (home)' > .config/tmux/plugins/tpm
# InvalidLink: a symlink to some other existing file.
echo '# my old zshrc (home)' > .zshrc.mine
ln -s .zshrc.mine .zshrc
# InvalidLink: a broken symlink.
ln -s nowhere/gitconfig .gitconfig
# InvalidLink: a link dots created earlier from the overridden auto rule
# (config/nvim), before [dots] "~/.config/nvim" = "nvim" was added.
ln -s ../../repo/config/nvim .config/nvim
# FileOrDir in the second auto rule.
echo 'echo "my old hello (home)"' > .local/bin/hello
# ValidLink: already linked the way dots would link it (skipped, not a conflict).
ln -s ../../repo/starship/linux.toml .config/starship.toml
