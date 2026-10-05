# Shortener

A Go 1.24 URL shortener using only the standard library.

```sh
go build -o shortener .
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

`ADMIN_TOKEN` must be non-empty. The data directory must already exist and be
writable. An absent data file is initialized to an empty JSON array; invalid
existing data causes startup to fail rather than silently discard links.
Run only one service process against a given data file.

```sh
curl -X POST localhost:8080/api/links \
  -d '{"url":"https://example.com/page","alias":"my-page"}'
curl -i localhost:8080/my-page
curl -H 'Authorization: Bearer secret' localhost:8080/api/links
curl -X DELETE -H 'Authorization: Bearer secret' localhost:8080/api/links/my-page
```

The API returns JSON errors, validates strict request fields, and limits create
bodies to 1 MiB. Admin list and delete operations require the exact bearer token.
Unsupported methods return 405 (including HEAD). Lists are ordered oldest first.

Every mutation, including visits, is serialized and saved before responding.
Persistence writes a private temporary file in the data directory, syncs it,
atomically replaces the JSON file, and syncs the directory. A killed process
leaves either the old or new complete file; abandoned temporary files are not
used on startup. Filesystem write failures return 500. This assumes a local
filesystem providing atomic rename and file/directory fsync semantics.

```sh
go test ./...
go vet ./...
# Optional concurrency diagnostics:
go test -race ./...
```
