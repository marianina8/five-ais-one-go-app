# URL Shortener Service

A simple URL shortener service written in Go with the standard library only.

## Features

- Create short links with custom aliases or auto-generated codes
- Redirect to original URLs with visit tracking
- Admin endpoints for listing and deleting links
- JSON file storage with atomic updates
- Concurrent request handling with proper locking
- Admin token authentication

## Building

```bash
go build -o shortener .
```

## Running

```bash
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

### Command-line flags

- `-addr`: Server address (default: `:8080`)
- `-data`: Data file path (default: `data.json`)

### Environment variables

- `ADMIN_TOKEN`: Required admin token for protected endpoints

## API

### Create a link

**POST** `/api/links`

Request body:
```json
{
  "url": "https://example.com/page",
  "alias": "my-page"  // optional
}
```

Response (201 Created):
```json
{
  "code": "my-page",
  "url": "https://example.com/page",
  "created_at": "2024-01-01T12:00:00Z",
  "visits": 0
}
```

### Follow a link

**GET** `/{code}`

Redirects (302 Found) to the stored URL and increments the visit counter.

### List all links (admin)

**GET** `/api/links`

Requires: `Authorization: Bearer <ADMIN_TOKEN>`

Response (200 OK):
```json
[
  {
    "code": "my-page",
    "url": "https://example.com/page",
    "created_at": "2024-01-01T12:00:00Z",
    "visits": 5
  }
]
```

### Delete a link (admin)

**DELETE** `/api/links/{code}`

Requires: `Authorization: Bearer <ADMIN_TOKEN>`

Response: 204 No Content

## Error responses

All error responses are in JSON format:
```json
{
  "error": "Error message"
}
```

### Status codes

- 200: Success
- 201: Link created
- 204: Link deleted
- 302: Redirect
- 400: Bad request (invalid input)
- 401: Unauthorized (missing/wrong admin token)
- 404: Not found
- 409: Conflict (alias already taken)
- 413: Request body too large

## Data storage

Links are stored in a JSON file specified by the `-data` flag. The file is updated atomically using a temporary file and rename operation to ensure data integrity even during crashes.

## Concurrency

The service uses proper read-write locks to handle concurrent requests safely. Visit counters are incremented atomically.

## Testing

Run all tests:

```bash
go test ./...
```

Run with verbose output:

```bash
go test -v ./...
```