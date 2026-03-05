package templates

import "embed"

// FS holds all embedded app templates.
//
//go:embed all:adguard all:beszel all:code-server all:gitea all:gitlab all:hedgedoc all:immich all:keycloak all:mastodon all:matrix all:mattermost all:nextcloud all:nginx-proxy-manager all:obsidian all:openclaw all:paperless-ngx all:portainer all:webtop
var FS embed.FS
