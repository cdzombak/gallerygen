#!/usr/bin/env bash

# Interactive script to set all required GitHub Actions secrets for this repo

set -euo pipefail

echo "This script will prompt you for each required secret and set it using 'gh secret set'."
echo

# Get repo name (owner/repo)
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)

# List of secrets and their descriptions
secrets=(
  "DOCKERHUB_USERNAME:Docker Hub username"
  "DOCKERHUB_TOKEN:Docker Hub access token"
  "HOMEBREW_RELEASER_PAT:GitHub Personal Access Token with repo scope for Homebrew formula updates"
  "NTFY_TOKEN:Bearer token for ntfy.cdzombak.net notifications"
  "APTLY_CRED:Credentials for Aptly repository (format: username:password)"
  "APTLY_API:URL for Aptly API endpoint"
  "TS_OAUTH_CLIENT_ID:Tailscale OAuth client ID for GitHub Actions"
  "TS_OAUTH_SECRET:Tailscale OAuth secret for GitHub Actions"
)

for secret_entry in "${secrets[@]}"; do
  secret_name="${secret_entry%%:*}"
  desc="${secret_entry#*:}"
  echo
  read -rsp "Enter value for $secret_name ($desc): " value
  echo
  if [[ -z "$value" ]]; then
    echo "Skipping $secret_name (no value entered)"
    continue
  fi
  echo -n "$value" | gh secret set "$secret_name" --repo "$REPO"
  echo "Set $secret_name"
done

echo
echo "All secrets processed."
