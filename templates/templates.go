package templates

import "embed"

// FS holds all embedded app templates.
//
//go:embed all:adguard all:audiobookshelf all:beszel all:bookstack all:calibre-web all:code-server all:crowdsec all:excalidraw all:gitea all:gitlab all:grafana all:hedgedoc all:home-assistant all:immich all:jellyfin all:keycloak all:mastodon all:matrix all:mattermost all:minio all:navidrome all:nextcloud all:nginx-proxy-manager all:ntfy all:obsidian all:openclaw all:paperless-ngx all:portainer all:stirling-pdf all:syncthing all:uptime-kuma all:vaultwarden all:vikunja all:webtop all:wireguard
var FS embed.FS
