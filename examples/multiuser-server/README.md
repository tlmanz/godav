# Multi-user Upload Server (Sample)

A minimal HTTP server demonstrating how to use godav with multiple users, per-user configs, UploadManager sessions, and SSE progress/events.

## Endpoints

- POST /api/uploads/start
  - Body: { userId, baseURL, davUser, davPass, localPath, remotePath }
  - Returns: { sessionId }
- POST /api/uploads/{id}/pause
- POST /api/uploads/{id}/resume
- DELETE /api/uploads/{id}
- GET /api/uploads/{id}/status => { status }
- GET /api/users/{userId}/stream => Server-Sent Events (progress/event JSON payloads)

Progress events:
- event: progress (ProgressInfo JSON)
- event: event (EventInfo JSON)

## Run

Go 1.23+ required.

```sh
cd examples/multiuser-server
go run .
```

In another terminal, start an upload:

```sh
curl -s -X POST localhost:8080/api/uploads/start \
 -H 'Content-Type: application/json' \
 -d '{
  "userId": "alice",
  "baseURL": "https://nextcloud.example.com/remote.php/dav/",
  "davUser": "alice",
  "davPass": "<app-password>",
  "localPath": "/path/to/local/file.bin",
  "remotePath": "Uploads/file.bin"
 }'
```

Stream events (in a browser or curl):

```sh
curl -N localhost:8080/api/users/alice/stream
```

Pause/resume:

```sh
curl -X POST localhost:8080/api/uploads/<sessionId>/pause
curl -X POST localhost:8080/api/uploads/<sessionId>/resume
```

Cancel and check status:

```sh
curl -X DELETE localhost:8080/api/uploads/<sessionId>
curl localhost:8080/api/uploads/<sessionId>/status
```

## Notes

- This sample uses the client’s default config. You can customize ChunkSize/MaxRetries/CheckpointFunc in SetConfig.
- For production, secure endpoints (authn/z), validate inputs, and stage localPath safely (e.g., after receiving a file).
- For persistence across restarts, persist checkpoints via CheckpointFunc and reload to resume.
