# Links every rule with dots apply (which records them in the state file),
# then removes three rules from dots.toml so their links become stale.
# Runs with HOME and REPO set; cwd is the repository.
set -eu
dots apply > /dev/null

cat > dots.toml <<'EOF'
# Scenario "stale": setup.sh linked ~/.vimrc, ~/.zshrc and ~/.tmux.conf too,
# then removed their rules. Only ~/.gitconfig is still declared.

[dots]
"~/.gitconfig" = "gitconfig"
EOF

cd "$HOME"
# StaleChanged: the user replaced the link dots created with a regular file.
rm .zshrc
echo '# my own zshrc (home)' > .zshrc
# StaleGone: the link dots created is already gone.
rm .tmux.conf
# ~/.vimrc stays as dots created it (StaleLink).
