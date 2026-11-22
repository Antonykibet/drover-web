# Drover Profit — landing page

Static marketing / waitlist page for **Drover Profit**: project profitability for design studios.

## Structure

```
index.html          # the whole page: design tokens, components, markup, waitlist script
shots/              # photography and product screenshots used on the page
Dockerfile          # nginx:alpine serving index.html + shots/
docker-compose.yml  # container + Traefik labels
.github/workflows/  # CI: image build on every push, deploy on main
```

No build step. Open it locally with any static server:

```sh
python3 -m http.server 8000
# then visit http://localhost:8000
```

Or the production container: `docker build -t drover-web . && docker run --rm -p 8080:80 drover-web`.

## Deployment

Runs on the VM as a container behind Traefik, which terminates TLS (Let's Encrypt,
`le` resolver) for the hostname in `DROVER_WEB_DOMAIN`.

Every push builds the image; a push to `main` then SSHes to the VM, pulls and runs
`docker compose up --build -d`. Repository secrets: `VM_HOST`, `VM_USER`, `VM_SSH_KEY`.

First-time setup on the VM (the `traefik-public` network and Traefik must already exist):

```sh
git clone <this repo> /apps/drover-web && cd /apps/drover-web
cp .env.example .env      # set DROVER_WEB_DOMAIN to the real hostname
docker compose up --build -d
```

Point an `A` record for that hostname at the VM.

## Notes

- All styling resolves to CSS custom properties defined in `:root` (the "Drover design system v2").
- The waitlist form currently writes through the Claude artifact runtime (`window.claude.use('db')`).
  Outside that runtime it shows a "nothing was saved" notice — wire it to a real backend before launch.
