# homelabctl

**Deploy self-hosted apps in seconds. Standard docker-compose underneath. Auto-generated dashboard on top.**

A single Go binary that deploys and manages Docker-based homelab applications with production-quality defaults, full transparency, and zero lock-in.

---

## Quick Demo

```
$ homelabctl deploy nextcloud

  Nextcloud Deployment
  --------------------
  Domain:       cloud.home.lab
  Admin user:   admin
  Admin pass:   [auto-generated]
  DB password:  [auto-generated]
  Backup path:  /var/lib/nextcloud

  Deploy? [Y/n] y

  [+] Generating docker-compose.yaml
  [+] Creating borgmatic config
  [+] Starting containers
  [+] Health check passed

  Nextcloud is running at https://cloud.home.lab
```

What you get: a standard `docker-compose.yaml` and `borgmatic` config in `/opt/homelabctl/apps/nextcloud/`. No magic, no proprietary formats.

## Installation

**From releases (recommended):**

```sh
curl -fsSL https://raw.githubusercontent.com/jdillenberger/homelabctl/main/install.sh | bash
```

**From source:**

```sh
go install github.com/jdillenberger/homelabctl/cmd/homelabctl@latest
```

## Quickstart

```sh
# 1. Initial setup -- installs dependencies, configures defaults
homelabctl setup

# 2. Validate your environment
homelabctl doctor

# 3. Deploy an app
homelabctl deploy nextcloud

# 4. Check status
homelabctl status

# 5. Open the auto-generated dashboard
homelabctl serve
```

## Key Features

**Deploy** -- Production-quality in one command. `homelabctl deploy nextcloud` runs an interactive wizard, generates secrets, writes docker-compose with security defaults, configures backups, and starts the stack.

**Discover** -- Auto-generated per-server portal dashboard that stays in sync with actual deployments. mDNS-based cross-server index for multi-node homelabs. Think Homer, but it never goes stale.

**Transparent** -- Everything is standard docker-compose + borgmatic. Run `cat` on any generated file. `docker compose` works directly. `homelabctl eject` if you want to leave.

**Secure defaults** -- Generated compose files include `security_opt`, log rotation, `pids_limit`, resource limits, and healthchecks out of the box.

**Backup orchestration** -- Each template declares its backup paths in `app.yaml`. `homelabctl backup create` knows what to back up without configuration.

**Export/Import** -- `homelabctl export` and `homelabctl import` for migration and disaster recovery.

**Image pinning** -- Resolve and pin image tags via registry APIs. No more `:latest` surprises.

## How It Works

homelabctl is built on three standard tools and one metadata layer:

```
app.yaml          -- Template metadata: typed values, secrets, requirements, backup paths, health URLs
docker-compose    -- Container orchestration (generated, standard, editable)
borgmatic         -- Backup configuration (generated, standard, editable)
```

When you run `homelabctl deploy <app>`:

1. The `app.yaml` metadata drives an interactive TUI wizard (built with charmbracelet/huh)
2. User inputs + auto-generated secrets are injected into Go templates
3. A standard `docker-compose.yaml` is written to `/opt/homelabctl/apps/<app>/`
4. A borgmatic config is generated based on declared backup paths
5. `docker compose up -d` starts the stack
6. Health checks confirm the deployment

Every generated file is a plain text file you can read, edit, or use independently.

## Template Customization

Override any template without forking using the OverlayFS-style system:

```sh
# Export a template for customization
homelabctl templates export nextcloud

# Edit the local override
vim ~/.config/homelabctl/templates/nextcloud/docker-compose.yaml.tmpl

# Deploy uses your override automatically
homelabctl deploy nextcloud
```

Local overrides take precedence. Unmodified files fall through to built-in defaults.

Create a new template from scratch:

```sh
homelabctl templates new myapp
```

## Available Templates

| Template | Description |
|---|---|
| adguard | DNS ad blocker |
| beszel | Server monitoring |
| code-server | VS Code in the browser |
| gitea | Lightweight Git hosting |
| gitlab | Full Git platform |
| hedgedoc | Collaborative markdown |
| immich | Photo management |
| keycloak | Identity and access management |
| mastodon | Federated social network |
| matrix | Decentralized messaging |
| mattermost | Team messaging |
| nextcloud | File sync and collaboration |
| nginx-proxy-manager | Reverse proxy with UI |
| obsidian | Note-taking (web sync) |
| openclaw | Legal document management |
| paperless-ngx | Document management |
| portainer | Container management UI |
| webtop | Linux desktop in the browser |

## Web Dashboard

```sh
homelabctl serve
```

Starts an auto-generated portal at `http://<server>:8420` showing all deployed apps with their status, health, and links. The dashboard is regenerated from actual deployment state -- it cannot drift.

For multi-server setups, `homelabctl fleet discover` uses mDNS to find other homelabctl instances and build a cross-server index.

## Configuration

```sh
# Show current config
homelabctl config show

# Set a value
homelabctl config set domain home.lab

# Validate config
homelabctl config validate
```

Config locations (in order of precedence):

- `/etc/homelabctl/config.yaml` -- system-wide
- `~/.config/homelabctl/config.yaml` -- per-user

## CLI Reference

```
homelabctl setup                  Initial setup wizard
homelabctl doctor                 Validate environment and dependencies
homelabctl deploy <app>           Deploy an app interactively
homelabctl apps list              List deployed apps
homelabctl apps info <app>        Show app details
homelabctl apps remove <app>      Remove an app
homelabctl apps status <app>      Show app status
homelabctl apps logs <app>        Tail app logs
homelabctl apps update <app>      Update app containers
homelabctl apps health <app>      Run health checks
homelabctl apps pin <app>         Pin image tags
homelabctl backup init            Initialize backup repository
homelabctl backup create          Create backup
homelabctl backup restore         Restore from backup
homelabctl backup list            List backups
homelabctl alerts list|add|remove Alert management
homelabctl fleet status           Multi-server fleet status
homelabctl fleet discover         Discover servers via mDNS
homelabctl templates list         List available templates
homelabctl serve                  Start web dashboard
homelabctl export                 Export for migration
homelabctl import                 Import from export
homelabctl eject                  Remove homelabctl, keep configs
homelabctl self-update            Update homelabctl binary
homelabctl completion             Shell completion scripts
```

## Ecosystem Positioning

**vs. Portainer / CasaOS / Yacht** -- homelabctl has smarter templates driven by metadata (`app.yaml`), better security defaults, and is CLI-first. No web UI required to deploy.

**vs. plain docker-compose** -- homelabctl adds secret generation, requirement checking, a deploy wizard, and baked-in best practices. You still get plain docker-compose files.

**vs. Ansible / Terraform** -- homelabctl is simpler for single-app deployment. No inventory files, no state backends. One command, one app.

## Contributing

Contributions are welcome. Template contributions are especially low-barrier -- an `app.yaml` and a `docker-compose.yaml.tmpl` is all you need to add a new app.

```sh
homelabctl templates new myapp    # Scaffold a new template
```

See the existing templates in `templates/` for examples.

## License

TBD
