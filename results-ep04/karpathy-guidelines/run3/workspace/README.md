# URL Shortener Service

A lightweight URL shortener service written in Go, using only the standard library.

## Build and Run

```bash
# Set admin token
export ADMIN_TOKEN=your-secret-token

# Build
go build -o shortener .

# Run
./shortener -addr :8080 -data data.json
```

## API Endpoints

### Create a link
`POST /api/links`
```json
{
  "url": "https://example.com",
  "alias": "my-page"  // optional
}
```
Returns `201 Created` with link details or appropriate error.

### Follow a link
`GET /{code}`
Returns `302 Found` redirect to the stored URL.

### List all links (admin)
`GET /api/links`
Requires `Authorization: Bearer <ADMIN_TOKEN>` header.
Returns `200 OK` with array of all links.

### Delete a link (admin)
`DELETE /api/links/{code}`
Requires `Authorization: Bearer <ADMIN_TOKEN>` header.
Returns `204 No Content` on success.

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestHandler_CreateLink
```

## Storage

Links are stored in a JSON file specified by the `-data` flag. The file is updated atomically on every change (create, visit, delete) using a temporary file and rename operation.

## Concurrency

The service is thread-safe and handles concurrent requests correctly using read-write locks. All operations that modify data (create, increment visits, delete) save to disk before returning the response.

## Error Responses

All error responses are JSON with format:
```json
{"error": "error message"}
```