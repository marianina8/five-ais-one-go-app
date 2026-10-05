# Shortener

Standard-library Go 1.24 URL shortener.

```
go build -o shortener .
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

Create links with `POST /api/links`, follow with `GET /{code}`, and administer using `Authorization: Bearer secret` on `GET /api/links` and `DELETE /api/links/{code}`. The JSON storage file is an array of links. Changes are serialized and written using a synced temporary file, atomic rename, and directory sync. The data file's parent directory must exist. Run `go test ./...` and `go vet ./...` to check the project.
