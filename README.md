# Drover Profit — landing page

Static marketing / waitlist page for **Drover Profit**: project profitability for design studios.

## Structure

```
index.html          # the whole page: design tokens, components, markup, waitlist script
shots/              # photography and product screenshots used on the page
Dockerfile          # nginx:alpine serving index.html + shots/
nginx.conf          # caching, security headers, gzip, real 404s
deploy/             # VM side: setup script, deploy/logs commands, compose file
.github/workflows/  # CI: asset check, image build, smoke test, deploy on main
```

No build step. Open it locally with any static server:

```sh
python3 -m http.server 8000
# then visit http://localhost:8000
```

Or the production container: `docker build -t drover-web . && docker run --rm -p 8080:80 drover-web`.

## Deployment

Runs on the VM as a container behind Traefik, which terminates TLS with a Let's
Encrypt certificate (`le` resolver). `www.<domain>` permanently redirects to `<domain>`.

Every push runs CI: it checks every `shots/` image the page references exists,
builds the image and smoke-tests it. A push to `main` then SSHes to the VM as the
`drover` user, whose key can only run `drover-web-deploy`: it pulls `main` into
`/srv/drover-web`, builds, restarts the container and fails unless it reports
healthy. Repository secrets: `VM_HOST`, `VM_USER` (`drover`), `VM_SSH_KEY`.

### The `drover` user

`drover` has no sudo or docker group. Through sudo it can run exactly two
root-owned commands:

```sh
sudo drover-web-deploy     # deploy main now
sudo drover-web-logs [-f]  # container logs
```

What the VM runs is fixed by root-owned files, not the repo: the compose file
(volumes, network, Traefik labels) is `/etc/drover-web/compose.yml`, and a deploy
refuses an image that sets its own `traefik.*` labels. Commits change the page
and its image, never routing or host access. Changes under `deploy/` take effect
only when root re-runs the setup.

### First-time setup on the VM

As the admin user, make a key for CI and run the setup as root:

```sh
ssh-keygen -t ed25519 -N '' -C gh-actions-deploy@drover-web -f ~/.ssh/drover_web_deploy
sudo bash deploy/setup.sh <domain> ~/.ssh/drover_web_deploy.pub [people.pub]
```

`people.pub` holds the public keys that get a shell as `drover`. The setup is
re-runnable and rewrites `drover`'s `authorized_keys` from its arguments each
time. Put the private key in the `VM_SSH_KEY` secret, and point `A` records for
the domain and `www.` at the VM before the first deploy.

## Notes

- All styling resolves to CSS custom properties defined in `:root` (the "Drover design system v2").
- The waitlist form currently writes through the Claude artifact runtime (`window.claude.use('db')`).
  Outside that runtime it shows a "nothing was saved" notice — wire it to a real backend before launch.
