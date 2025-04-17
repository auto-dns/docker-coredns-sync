#!/usr/bin/env bash
set -e

# echo "Downloading go module dependencies..."
# go mod download

grep -q "export HISTFILE=" ~/.zshrc || echo 'export HISTFILE=/home/vscode/.shell_history/zsh_history' >> ~/.zshrc

sudo chown -R vscode:vscode /home/vscode/go /home/vscode/.cache/go-build

echo "Post-create tasks completed."
