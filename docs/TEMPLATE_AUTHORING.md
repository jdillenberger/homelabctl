# Template Authoring Guide

This document describes how to create new app templates for homelabctl.

## Directory Structure

Each template lives in its own directory and must contain exactly three files:

```
myapp/
  app.yaml                 # App metadata and configuration schema
  docker-compose.yml.tmpl  # Go text/template for the Docker Compose file
  .env.tmpl                # Go text/template for the environment file
```

**Built-in templates** ship in `templates/` inside the homelabctl binary.
**User templates** live under `~/.homelabctl/templates/`.
User templates with the same directory name as a built-in template override
the built-in version entirely (see [OverlayFS Customization](#overlayfs-customization)).

---

## app.yaml Schema

The `app.yaml` file defines all metadata, ports, volumes, values, health
checks, backup settings, hooks, requirements, and dependencies for the app.

```yaml
name: string          # App name (defaults to directory name if omitted)
description: string   # Short description
category: string      # e.g. "infra", "productivity", "media", "dev"
version: string       # Template version

ports:
  - host: int              # Host port
    container: int         # Container port
    protocol: string       # "tcp" or "udp"
    description: string    # What this port is for
    value_name: string     # Optional: name of the Value that sets the host port

volumes:
  - name: string           # Subdirectory name under data_dir
    container: string      # Mount path inside container
    description: string    # What this volume stores

values:
  - name: string           # Value name (used in templates as {{.name}})
    description: string    # Shown in wizard/docs
    default: string        # Default value
    required: bool         # Must be set before deploy
    secret: bool           # Masked in TUI wizard
    auto_gen: string       # "password" (32-char hex) or "uuid" -- auto-generated if empty

health_check:
  url: string              # Go template URL, e.g. "http://localhost:{{.web_port}}"
  interval: string         # Check interval, e.g. "30s"

backup:
  paths: [string]          # Paths to back up (default: data_dir/app_name)
  pre_hook: string         # Shell command to run before backup (e.g. DB dump)
  post_hook: string        # Shell command to run after backup

hooks:
  post_deploy:             # Hooks to run after successful deployment
    - type: string         # "http" or "exec"
      url: string          # For http hooks: URL template
      method: string       # For http hooks: HTTP method
      body: string         # For http hooks: request body template
      command: string      # For exec hooks: shell command template
  pre_remove:              # Hooks to run before app removal
    - type: string
      command: string

requirements:
  min_ram: string          # e.g. "2GB"
  min_disk: string         # e.g. "10GB"
  arch: [string]           # e.g. ["amd64", "arm64"]

dependencies: [string]     # Other apps that must be deployed first
```

---

## Template Values

### Standard values (always available)

The following values are injected automatically and can be used in any
template without declaring them in the `values` section of `app.yaml`:

| Value       | Description                              |
|-------------|------------------------------------------|
| `hostname`  | Server hostname                          |
| `domain`    | Network domain (default `"local"`)       |
| `data_dir`  | App's data directory path                |
| `app_name`  | The app name                             |
| `network`   | Docker network name                      |
| `timezone`  | Defaults to `"UTC"` if not set           |

### Custom values

Any additional values your template needs should be declared in the `values`
list. Each value becomes available in templates via `{{index . "name"}}`.

- Set `secret: true` for passwords and tokens so the TUI wizard masks input.
- Set `auto_gen: password` to auto-generate a 32-character hex string when the
  value is left empty.
- Set `auto_gen: uuid` to auto-generate a UUID when the value is left empty.

---

## Template Syntax

Template files (`.tmpl`) use Go's `text/template` package with
`missingkey=error` -- referencing an undefined value is a hard error, not a
silent empty string.

### Accessing values

Use the `index` function to access values by name:

```
{{index . "web_port"}}
{{index . "data_dir"}}
{{index . "hostname"}}.{{index . "domain"}}
```

### Available template functions

| Function      | Description                          | Example                                      |
|---------------|--------------------------------------|----------------------------------------------|
| `default`     | Provide a fallback value             | `{{default "3000" .web_port}}`               |
| `genPassword` | Generate a random 32-char hex string | `{{genPassword}}`                            |
| `genUUID`     | Generate a UUID                      | `{{genUUID}}`                                |
| `upper`       | Convert string to uppercase          | `{{upper (index . "db_name")}}`              |
| `lower`       | Convert string to lowercase          | `{{lower (index . "app_name")}}`             |
| `replace`     | String replacement                   | `{{replace "." "-" (index . "hostname")}}`   |

---

## Examples

### Minimal example: AdGuard Home

A simple single-container app with no database and no secrets.

**app.yaml:**

```yaml
name: adguard
description: Network-wide ad and tracker blocking DNS server
category: infra
version: "v0.107.72"
ports:
  - host: 3000
    container: 3000
    protocol: tcp
    description: Web UI (initial setup)
    value_name: web_port
  - host: 80
    container: 80
    protocol: tcp
    description: Web UI (after setup)
  - host: 53
    container: 53
    protocol: udp
    description: DNS
    value_name: dns_port
values:
  - name: web_port
    description: Web UI port
    default: "3000"
  - name: dns_port
    description: DNS port
    default: "53"
  - name: timezone
    description: Timezone
    default: "UTC"
dependencies:
  - docker
volumes:
  - name: work
    container: /opt/adguardhome/work
    description: AdGuard working data
  - name: conf
    container: /opt/adguardhome/conf
    description: AdGuard configuration
health_check:
  url: "http://localhost:{{.web_port}}"
  interval: 30s
backup:
  paths:
    - conf
    - work
requirements:
  min_ram: 128M
  arch: [amd64, arm64, armv7]
```

**docker-compose.yml.tmpl:**

```
services:
  adguard:
    image: adguard/adguardhome:v0.107.72
    container_name: adguard
    restart: unless-stopped
    ports:
      - "{{index . "web_port"}}:3000/tcp"
      - "{{index . "dns_port"}}:53/tcp"
      - "{{index . "dns_port"}}:53/udp"
    environment:
      - TZ={{index . "timezone"}}
    deploy:
      resources:
        limits:
          memory: 256M
    volumes:
      - {{index . "data_dir"}}/work:/opt/adguardhome/work
      - {{index . "data_dir"}}/conf:/opt/adguardhome/conf
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider http://localhost:3000 || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    security_opt:
      - no-new-privileges:true
    pids_limit: 200
    networks:
      - {{index . "network"}}

networks:
  {{index . "network"}}:
    external: true
```

**.env.tmpl:**

```
# AdGuard Home environment
WEB_PORT={{index . "web_port"}}
DNS_PORT={{index . "dns_port"}}
```

### Complex example: Nextcloud with database, secrets, and backup hooks

A multi-container app with MariaDB, auto-generated secrets, and database
backup hooks.

**app.yaml:**

```yaml
name: nextcloud
description: Self-hosted productivity and file sync platform
category: productivity
version: "32.0.6"
ports:
  - host: 8080
    container: 80
    protocol: tcp
    description: Web UI
    value_name: web_port
values:
  - name: web_port
    description: Web UI port
    default: "8080"
  - name: db_password
    description: Database root password
    secret: true
    auto_gen: password
  - name: nextcloud_admin_user
    description: Nextcloud admin username
    default: "admin"
  - name: nextcloud_admin_password
    description: Nextcloud admin password
    secret: true
    auto_gen: password
  - name: db_name
    description: Database name
    default: "nextcloud"
  - name: db_user
    description: Database user
    default: "nextcloud"
  - name: db_user_password
    description: Database user password
    secret: true
    auto_gen: password
  - name: timezone
    description: Timezone
    default: "UTC"
dependencies:
  - docker
volumes:
  - name: html
    container: /var/www/html
    description: Nextcloud application files
  - name: db
    container: /var/lib/mysql
    description: MariaDB data
health_check:
  url: "http://localhost:{{.web_port}}"
  interval: 60s
backup:
  paths:
    - html
    - db
  pre_hook: >-
    docker exec nextcloud-db sh -c
    'mariadb-dump -u root -p"$MYSQL_ROOT_PASSWORD" --all-databases'
    > /tmp/nextcloud-db-backup.sql
  post_hook: "rm -f /tmp/nextcloud-db-backup.sql"
requirements:
  min_ram: 512M
  min_disk: 10G
  arch: [amd64, arm64]
```

**docker-compose.yml.tmpl:**

```
services:
  nextcloud:
    image: nextcloud:32.0.6
    container_name: nextcloud
    restart: unless-stopped
    ports:
      - "{{index . "web_port"}}:80"
    environment:
      - TZ={{index . "timezone"}}
      - MYSQL_HOST=nextcloud-db
      - MYSQL_DATABASE={{index . "db_name"}}
      - MYSQL_USER={{index . "db_user"}}
      - MYSQL_PASSWORD={{index . "db_user_password"}}
      - NEXTCLOUD_ADMIN_USER={{index . "nextcloud_admin_user"}}
      - NEXTCLOUD_ADMIN_PASSWORD={{index . "nextcloud_admin_password"}}
      - NEXTCLOUD_TRUSTED_DOMAINS={{index . "hostname"}}.{{index . "domain"}}
      - OVERWRITEPROTOCOL=https
    healthcheck:
      test: ["CMD-SHELL", "curl -f http://localhost:80/status.php || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
    deploy:
      resources:
        limits:
          memory: 1G
    volumes:
      - {{index . "data_dir"}}/html:/var/www/html
    depends_on:
      nextcloud-db:
        condition: service_healthy
    security_opt:
      - no-new-privileges:true
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    pids_limit: 200
    networks:
      - {{index . "network"}}

  nextcloud-db:
    image: mariadb:11.4
    container_name: nextcloud-db
    restart: unless-stopped
    shm_size: '256m'
    environment:
      - TZ={{index . "timezone"}}
      - MYSQL_ROOT_PASSWORD={{index . "db_password"}}
      - MYSQL_DATABASE={{index . "db_name"}}
      - MYSQL_USER={{index . "db_user"}}
      - MYSQL_PASSWORD={{index . "db_user_password"}}
    deploy:
      resources:
        limits:
          memory: 512M
    volumes:
      - {{index . "data_dir"}}/db:/var/lib/mysql
    healthcheck:
      test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
      interval: 10s
      timeout: 5s
      retries: 5
    security_opt:
      - no-new-privileges:true
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    pids_limit: 100
    networks:
      - {{index . "network"}}

networks:
  {{index . "network"}}:
    external: true
```

**.env.tmpl:**

```
# Nextcloud environment
WEB_PORT={{index . "web_port"}}
DB_NAME={{index . "db_name"}}
DB_USER={{index . "db_user"}}
```

Key things to note in this example:

- Three values use `secret: true` and `auto_gen: password` so credentials are
  auto-generated and masked in the TUI wizard.
- The `depends_on` with `condition: service_healthy` ensures Nextcloud waits
  for MariaDB to be ready.
- The `backup.pre_hook` dumps the database before backup, and `post_hook`
  cleans up the dump file afterwards.
- Both containers have separate resource limits, healthchecks, and PID limits.

---

## OverlayFS Customization

Users can override any built-in template by exporting it to the local
templates directory:

```
homelabctl templates export nextcloud
```

This copies the entire built-in `nextcloud` template to
`~/.homelabctl/templates/nextcloud/`, where all three files can be freely
edited. The local directory overrides the built-in version completely --
homelabctl will use only the files from `~/.homelabctl/templates/nextcloud/`
and ignore the built-in ones.

This is useful for:

- Changing default port numbers or resource limits
- Adding extra containers (e.g. a Redis sidecar)
- Modifying environment variables for your specific setup
- Adjusting healthcheck parameters

If you want to revert to the built-in version, simply delete the directory
under `~/.homelabctl/templates/`.

---

## Template Scaffolding

To create a new template from scratch, use the scaffolding command:

```
homelabctl templates new myapp
```

This creates a skeleton in `~/.homelabctl/templates/myapp/` with starter
versions of all three required files:

- `app.yaml` -- pre-filled with common fields and placeholder values
- `docker-compose.yml.tmpl` -- a single-service template with security
  hardening already in place
- `.env.tmpl` -- a minimal environment file

Edit these files to match your app's requirements, then deploy with:

```
homelabctl deploy myapp
```

---

## Tips and Best Practices

### Security hardening

Every container should include these security options:

```yaml
security_opt:
  - no-new-privileges:true
```

This prevents processes inside the container from gaining additional
privileges via setuid/setgid binaries.

Where the application supports it, use a read-only root filesystem:

```yaml
read_only: true
tmpfs:
  - /tmp
  - /run
```

### Logging limits

Always set logging limits to prevent disk exhaustion from runaway logs:

```yaml
logging:
  driver: json-file
  options:
    max-size: "10m"
    max-file: "3"
```

### PID limits

Set `pids_limit` to prevent fork bombs from consuming all system PIDs:

```yaml
pids_limit: 200
```

Choose a value appropriate for the app. Simple single-process apps may work
fine with 100; apps that spawn many workers may need 500 or more.

### Resource limits

Set memory and CPU limits so a misbehaving container cannot starve the host:

```yaml
deploy:
  resources:
    limits:
      memory: 512M
      cpus: "1.0"
```

### Healthchecks

Always define a `healthcheck` in the Docker Compose template. This enables
`depends_on` with `condition: service_healthy` for multi-container apps and
gives homelabctl accurate status information:

```yaml
healthcheck:
  test: ["CMD-SHELL", "curl -f http://localhost:80/ || exit 1"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

Adjust `start_period` for apps that take longer to initialize (databases,
Java applications, etc.).

### Multi-container apps

When your app needs a database or other sidecar:

- Define each service in the same `docker-compose.yml.tmpl`.
- Use `depends_on` with `condition: service_healthy` so the app waits for
  its dependencies.
- Give each container its own resource limits, healthcheck, and PID limit.
- Use separate volume subdirectories (e.g. `{{index . "data_dir"}}/db` and
  `{{index . "data_dir"}}/html`).

### Networking

All containers should join the shared homelabctl network:

```yaml
networks:
  - {{index . "network"}}
```

And declare it as external at the top level:

```yaml
networks:
  {{index . "network"}}:
    external: true
```

This allows inter-app communication (e.g. a reverse proxy reaching backend
apps) while keeping everything on a single Docker network.
