package templates

import "embed"

// FS holds all embedded app templates.
//
//go:embed all:actual-budget all:adguard all:audiobookshelf all:beszel all:bookstack all:calibre-web all:changedetection all:code-server all:crowdsec all:excalidraw all:firefly-iii all:forgejo all:ghost all:gitea all:gitlab all:grafana all:hedgedoc all:home-assistant all:immich all:jellyfin all:keycloak all:linkwarden all:mastodon all:matrix all:mattermost all:mealie all:minio all:navidrome all:nextcloud all:nginx-proxy-manager all:ntfy all:obsidian all:openclaw all:paperless-ngx all:portainer all:searxng all:stirling-pdf all:syncthing all:traefik all:umami all:uptime-kuma all:vaultwarden all:vikunja all:webtop all:wikijs all:wireguard all:wordpress
var FS embed.FS
