#!/bin/sh
set -eu

SERVERLIST_UID="${SERVERLIST_UID:-${PUID:-1000}}"
SERVERLIST_GID="${SERVERLIST_GID:-${PGID:-1000}}"
SERVERLIST_DATA_DIR="${SERVERLIST_DATA_DIR:-/var/lib/serverlist}"
SERVERLIST_GEOIP_DATABASE="${SERVERLIST_GEOIP_DATABASE:-$SERVERLIST_DATA_DIR/dbip-country-lite.mmdb}"
SERVERLIST_DOWNLOAD_GEOIP="${SERVERLIST_DOWNLOAD_GEOIP:-true}"
SERVERLIST_REQUIRE_GEOIP="${SERVERLIST_REQUIRE_GEOIP:-false}"

export SERVERLIST_DATA_DIR
export SERVERLIST_GEOIP_DATABASE

truthy() {
	case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" in
		1|true|yes|on) return 0 ;;
		*) return 1 ;;
	esac
}

ensure_user() {
	if ! getent group "$SERVERLIST_GID" >/dev/null 2>&1; then
		addgroup -S -g "$SERVERLIST_GID" serverlist
	fi
	group_name="$(getent group "$SERVERLIST_GID" | cut -d: -f1)"

	if ! getent passwd "$SERVERLIST_UID" >/dev/null 2>&1; then
		adduser -S -D -H -u "$SERVERLIST_UID" -G "$group_name" serverlist
	fi
	user_name="$(getent passwd "$SERVERLIST_UID" | cut -d: -f1)"
}

download_geoip() {
	if ! truthy "$SERVERLIST_DOWNLOAD_GEOIP"; then
		return 0
	fi
	if [ -s "$SERVERLIST_GEOIP_DATABASE" ]; then
		return 0
	fi

	mkdir -p "$(dirname "$SERVERLIST_GEOIP_DATABASE")"
	month="$(date -u +%Y-%m)"
	url="${SERVERLIST_GEOIP_URL:-https://download.db-ip.com/free/dbip-country-lite-$month.mmdb.gz}"
	tmp="$(mktemp)"
	trap 'rm -f "$tmp" "$tmp.gz"' EXIT

	echo "Downloading GeoIP database from $url"
	if wget -O "$tmp.gz" "$url" && gzip -dc "$tmp.gz" > "$tmp"; then
		mv "$tmp" "$SERVERLIST_GEOIP_DATABASE"
		return 0
	fi

	rm -f "$tmp" "$tmp.gz"
	if truthy "$SERVERLIST_REQUIRE_GEOIP"; then
		echo "GeoIP database download failed and SERVERLIST_REQUIRE_GEOIP is true." >&2
		return 1
	fi
	echo "GeoIP database download failed; continuing without GeoIP." >&2
}

finalize_geoip() {
	if [ -s "$SERVERLIST_GEOIP_DATABASE" ]; then
		export SERVERLIST_GEOIP_DATABASE
		return 0
	fi

	if truthy "$SERVERLIST_REQUIRE_GEOIP"; then
		echo "GeoIP database is missing and SERVERLIST_REQUIRE_GEOIP is true." >&2
		return 1
	fi

	echo "GeoIP database is missing; disabling GeoIP lookup." >&2
	unset SERVERLIST_GEOIP_DATABASE
}

mkdir -p "$SERVERLIST_DATA_DIR"
download_geoip
finalize_geoip
ensure_user
chown -R "$SERVERLIST_UID:$SERVERLIST_GID" "$SERVERLIST_DATA_DIR"

exec su-exec "$user_name:$group_name" "$@"
