#!/bin/bash
# One-time (re-runnable) host setup for drover-web. Run as root:
#
#   sudo bash deploy/setup.sh <domain> <ci-key.pub> [people-keys.pub]
#
# Creates the unprivileged `drover` user (no sudo group, no docker group) and
# lets it run exactly two root-owned commands: drover-web-deploy and
# drover-web-logs. The CI key is forced to run the deploy and nothing else; any
# keys in people-keys.pub get a normal shell as drover. authorized_keys is
# rewritten from the arguments on every run.
set -euo pipefail

[ "$(id -u)" = 0 ] || { echo "run as root" >&2; exit 1; }
[ $# -ge 2 ] || { sed -n '2,11p' "$0" >&2; exit 2; }

DOMAIN=$1
CI_PUB=$2
PEOPLE_PUB=${3:-}
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=https://github.com/Antonykibet/drover-web.git

[[ $DOMAIN =~ ^[A-Za-z0-9.-]+$ ]] || { echo "bad domain: $DOMAIN" >&2; exit 1; }
ssh-keygen -lf "$CI_PUB" >/dev/null
[ -z "$PEOPLE_PUB" ] || ssh-keygen -lf "$PEOPLE_PUB" >/dev/null

# User: no extra groups, no password (key-only).
id drover >/dev/null 2>&1 || useradd --create-home --shell /bin/bash drover
usermod -G "" drover

# Checkout, root-owned so drover cannot change what gets built.
[ -d /srv/drover-web/.git ] || git clone -q "$REPO" /srv/drover-web
chown -R root:root /srv/drover-web

# Host-owned compose file with the real domain.
install -d -m 755 /etc/drover-web
sed "s/DOMAIN/$DOMAIN/" "$HERE/compose.yml" > /etc/drover-web/compose.yml
chmod 644 /etc/drover-web/compose.yml

install -m 755 -o root -g root "$HERE/drover-web-deploy" /usr/local/bin/drover-web-deploy
install -m 755 -o root -g root "$HERE/drover-web-logs"   /usr/local/bin/drover-web-logs

# `""` means "no arguments allowed"; a bare path would allow any.
tmp=$(mktemp)
cat > "$tmp" <<'SUDOERS'
drover ALL=(root) NOPASSWD: /usr/local/bin/drover-web-deploy "", /usr/local/bin/drover-web-logs "", /usr/local/bin/drover-web-logs -f
SUDOERS
visudo -cf "$tmp" >/dev/null
install -m 440 -o root -g root "$tmp" /etc/sudoers.d/drover-web
rm -f "$tmp"

# Root-owned ~/.ssh so drover cannot add keys or lift the CI key's restriction.
install -d -m 755 -o root -g root /home/drover/.ssh
{
  printf 'restrict,command="sudo -n /usr/local/bin/drover-web-deploy" %s\n' "$(cat "$CI_PUB")"
  [ -z "$PEOPLE_PUB" ] || grep -E '^(ssh-|ecdsa-|sk-)' "$PEOPLE_PUB"
} > /home/drover/.ssh/authorized_keys
chown root:root /home/drover/.ssh/authorized_keys
chmod 644 /home/drover/.ssh/authorized_keys

# sshd must let drover in.
if sshd -T | grep -qiE '^allow(users|groups) '; then
  echo "WARNING: sshd has AllowUsers/AllowGroups; add drover there:" >&2
  sshd -T | grep -iE '^allow(users|groups) ' >&2
fi

echo "drover-web host setup done (domain: $DOMAIN)."
echo "Groups for drover: $(id -nG drover)"
