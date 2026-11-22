# Drover Profit — landing page

Static marketing / waitlist page for **Drover Profit**: project profitability for design studios.

## Structure

```
index.html          # the whole page: design tokens, components, markup, waitlist script
shots/              # photography and product screenshots used on the page
Dockerfile          # nginx:alpine serving index.html + shots/
nginx.conf          # caching, security headers, real 404s
docker-compose.yml  # container + Traefik labels for the VM
.github/workflows/  # CI: asset check, image build, smoke test, deploy on main
```

No build step. Open it locally with any static server:

```sh
python3 -m http.server 8000
# then visit http://localhost:8000
```

Or the production container: `docker build -t drover-web . && docker run --rm -p 8080:80 drover-web`.

## Deployment

Deployed to the VM as a container behind Traefik, the same way as `mtagger-web`
(see the infrastructure repo, `services/traefik/README.md`, *Connecting a Docker
service to Traefik*). nginx serves the page; Traefik terminates TLS with a Let's
Encrypt certificate from the `le` resolver and routes `DROVER_WEB_DOMAIN`
(a placeholder `drover.example.com` until set) to port 80. The `logging: promtail` label means the
monitoring stack's Alloy picks up its logs with no extra configuration.

Every push runs CI: it checks every `shots/` image the page references exists,
builds the image, and smoke-tests the container. A push to `main` then SSHes to the
host, pulls, rebuilds, and fails the job if the container doesn't report healthy.
It needs three repository secrets: `VM_HOST`, `VM_USER`, `VM_SSH_KEY`.

### First-time host setup

```sh
ssh <vm>
git clone <this repo> /apps/drover-web
cd /apps/drover-web
cp .env.example .env      # set DROVER_WEB_DOMAIN to the real hostname
docker compose up --build -d
```

`traefik-public` must already exist and the Traefik stack must be running — it is,
for everything else on the box. DNS: an `A` record for the chosen hostname pointing
at the host. Traefik requests the certificate on the first request once it resolves.

## Notes

- All styling resolves to CSS custom properties defined in `:root` (the "Drover design system v2").
- The waitlist form currently writes through the Claude artifact runtime (`window.claude.use('db')`).
  Outside that runtime it shows a "nothing was saved" notice — wire it to a real backend before launch.
