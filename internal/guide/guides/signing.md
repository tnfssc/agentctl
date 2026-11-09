# Set up git commit signing

You're helping the user sign their git commits and tags so GitHub shows them as Verified. agentctl doesn't manage
keys; you walk the user through it on this machine. Keys and accounts are theirs: ask before creating a key,
changing their global git config or adding anything to GitHub, and never print or copy a private key.

## Look first

- `git config --global --get-regexp '^(user\.signingkey|gpg\.|commit\.gpgsign|tag\.gpgsign)'`: what's set already.
- `ls ~/.ssh/*.pub`, and `ssh-add -L` for keys an agent (ssh-agent, 1Password, Secretive) already serves.
- `gh auth status`: whether `gh` can add the key (it needs the `admin:ssh_signing_key` scope).

If signing already works (`git log --show-signature -1` in any repo shows a good signature), say so and stop.

## SSH signing (the simple default)

1. Pick a key: an existing public key the user is happy to sign with, or a new one
   (`ssh-keygen -t ed25519 -C "<email> signing" -f ~/.ssh/id_ed25519_signing`). Agents commit without a person at
   the keyboard, so the key must be usable non-interactively: loaded in an agent, or held by a key manager
   (1Password: `gpg.ssh.program` = its `op-ssh-sign`). Explain that trade-off before choosing a key without a
   passphrase.
2. Configure git:
   ```sh
   git config --global gpg.format ssh
   git config --global user.signingkey ~/.ssh/<key>.pub
   git config --global commit.gpgsign true
   git config --global tag.gpgsign true
   ```
3. Let git verify its own signatures: write `<email> namespaces="git" <public key>` to
   `~/.config/git/allowed_signers` and set `gpg.ssh.allowedSignersFile` to it.
4. Register the key on GitHub as a signing key (separate from authentication keys):
   `gh auth refresh -s admin:ssh_signing_key`, then
   `gh ssh-key add ~/.ssh/<key>.pub --type signing --title "<machine> signing"`. Without `gh`: GitHub → Settings →
   SSH and GPG keys → New SSH key → Key type "Signing Key". The commit email must be a verified email on that account
   (`git config --global user.email`).

## Check

In a throwaway repo: `git init -q /tmp/signcheck && git -C /tmp/signcheck commit -q --allow-empty -m check &&
git -C /tmp/signcheck log --show-signature -1`, expecting "Good "git" signature". Then remove it. Tell the user that
GitHub shows Verified on commits pushed from now on.

## If it fails

- "Couldn't load public key" / "no such file": `user.signingkey` must point at the `.pub` file or be a `key::` literal.
- Signing hangs or asks for a passphrase: the key isn't in an agent; `ssh-add` it, or use the key manager's program.
- GPG instead of SSH: only if the user already uses GPG; set `gpg.format openpgp` and `user.signingkey` to the key id.
