# URL Shortener Service

A Go-based URL shortener service with persistent storage and admin functionality.

## Features

- Create shortened URLs with optional custom aliases
- Visit tracking for each link
- JSON file-based persistent storage
- Admin API with authentication
- Thread-safe concurrent operations
- Clean REST API

## Quick Start

```bash
# Set admin token
export ADMIN_TOKEN=your-secret-token

# Build the service
go build -o shortener .

# Run the service
./shortener -addr :8080 -data data.json
```

## API Endpoints

### Create a Link
```
POST /api/links
Content-Type: application/json

{
  "url": "https://example.com",
  "alias": "my-alias"  # optional
}
```

Response (201 Created):
```json
{
  "code": "AbC123d",
  "url": "https://example.com",
  "created_at": "2024-01-01T00:00:00Z",
  "visits": 0
}
```

### Follow a Link
```
GET /{code}
```
Redirects (302) to the original URL and increments visit count.

### List Links (Admin)
```
GET /api/links
Authorization: Bearer {ADMIN_TOKEN}
```

### Delete a Link (Admin)
```
DELETE /api/links/{code}
Authorization: Bearer {ADMIN_TOKEN}
```

## Configuration

- `-addr`: Server address (default: `:8080`)
- `-data`: JSON data file path (default: `data.json`)
- `ADMIN_TOKEN`: Required environment variable for admin operations

## Validation Rules

- URLs must be absolute `http` or `https` URLs with a host
- URLs must be ≤ 2048 characters
- Aliases must match `^[A-Za-z0-9_-]{3,32}$`
- Alias `api` is reserved
- Request bodies limited to 1 MiB

## Storage

- Data is persistently stored in a JSON file
- Atomic file operations ensure data integrity
- Automatic reload on restart
- Thread-safe for concurrent access

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run go vet for code quality
go vet ./...
```

## Build

```bash
go build -o shortener .
```