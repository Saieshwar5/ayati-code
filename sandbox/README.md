# Perpetual workspace sandbox

Build the local development image once:

```bash
docker build -t perpetual-sandbox:dev sandbox
```

Perpetual resolves this image to an immutable ID when it registers the default Local Docker environment. An active workspace exclusively leases one ready environment, and Perpetual creates a disposable container identified by that environment and lease generation. Only the leased workspace is mounted read-write at `/workspace`. `/tmp` and `/home/ayati` are writable tmpfs mounts. `/cache` is a writable managed directory outside the repository and survives runtime recreation and environment reassignment. Stop destroys the exact verified runtime before releasing the environment for another workspace. The Docker socket, host home, Fireworks key, GitHub credentials, and Perpetual database are not mounted.

The non-user-facing `/home/ayati` container path is retained as a runtime ABI so existing sandbox images and active containers remain verifiable during the product rename.

Workspace-specific environment values are injected for each shell command through a short-lived launcher. They are not baked into this image or saved in the container configuration. Values marked for setup are also available to dependency initialization.

The image intentionally provides a small common toolset for Go, Node.js, and Python projects. Project dependencies are installed during deterministic workspace preparation. Dependencies written into ignored project directories and the external package cache remain available when a stopped workspace resumes on available environment capacity.
