# Shortener

A Go 1.24 URL shortener using only the standard library.

```sh
go build -o shortener .
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

Both flags are optional; the defaults are shown. A non-empty `ADMIN_TOKEN` is
required. The data file's parent directory must already exist. A missing data
file starts an empty store; corrupt existing data causes startup to fail rather
than silently discarding links.

## API

- `POST /api/links`: public; JSON `{"url":"https://example.com","alias":"my-page"}`.
  Omit `alias` for a random seven-character alphanumeric code. Returns 201.
- `GET /{code}`: public; records a visit and returns a 302 redirect.
- `GET /api/links`: admin; returns all links, oldest first.
- `DELETE /api/links/{code}`: admin; deletes a link and returns 204.

Admin endpoints require `Authorization: Bearer <ADMIN_TOKEN>`. Errors and API
responses are JSON. Unsupported methods return 405 (including HEAD).

Storage is a JSON array of records with `code`, `url`, `created_at`, and `visits`.
Each mutation is synchronized, written to a same-directory temporary file,
flushed, atomically renamed over the live file, and followed by a directory
flush before responding. Readers never see a partial snapshot. Interrupted
writes may leave harmless `.shortener-*.tmp` files, which are ignored on startup.
Run only one service process per data file. Atomic replacement and directory
sync require a filesystem that supports those operations (as on local Unix
filesystems). Storage failures return 500; mutations failing before replacement
leave the old state intact.

```sh
go test ./...
go vet ./...
# Optional race-detector check on supported platforms:
go test -race ./...
```
