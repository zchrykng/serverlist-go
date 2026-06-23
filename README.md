# serverlist-go

Go port of the Luanti server list service.

## Development

The Go application source lives under `src/`.

Run tests:

```sh
go test ./...
```

Run locally:

```sh
go run ./src
```

## Docker

Build the image:

```sh
docker build -t serverlist-go .
```

Run it with a persistent data directory:

```sh
docker run --rm \
  -p 5000:5000 \
  -v serverlist-data:/var/lib/serverlist \
  serverlist-go
```

Runtime environment variables:

- `SERVERLIST_HOST`: HTTP bind address. Defaults to `0.0.0.0` in Docker.
- `SERVERLIST_PORT`: HTTP port. Defaults to `5000`.
- `SERVERLIST_DATA_DIR`: directory for `store.json` and `list.json`. Defaults to `/var/lib/serverlist`.
- `SERVERLIST_UID` / `SERVERLIST_GID`: runtime user and group IDs. Defaults to `1000`.
- `PUID` / `PGID`: aliases for `SERVERLIST_UID` / `SERVERLIST_GID`.
- `SERVERLIST_GEOIP_DATABASE`: MMDB path. Defaults to `/var/lib/serverlist/dbip-country-lite.mmdb`.
- `SERVERLIST_DOWNLOAD_GEOIP`: download the DB-IP country Lite MMDB if missing. Defaults to `true`.
- `SERVERLIST_GEOIP_URL`: override the MMDB `.gz` download URL.
- `SERVERLIST_REQUIRE_GEOIP`: fail startup if GeoIP download fails. Defaults to `false`.
- `SERVERLIST_REJECT_PRIVATE_ADDRESSES`: override private address rejection.

The default GeoIP URL uses the current UTC month:

```text
https://download.db-ip.com/free/dbip-country-lite-YYYY-MM.mmdb.gz
```

## GitHub Actions

The repository includes two workflows:

- `CI`: runs `go test ./...` and verifies the Docker image builds on pull requests and pushes to `main` or `master`.
- `Docker Publish`: builds and pushes a multi-arch image to GitHub Container Registry as `ghcr.io/<owner>/<repo>` on pushes to `main` or `master`, semver tags, published releases, and manual dispatches.

The publish workflow uses the built-in `GITHUB_TOKEN` with `packages: write`, so no registry secret is needed for GHCR in the same repository.
