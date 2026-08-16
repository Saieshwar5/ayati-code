# Ayati workspace sandbox

Build the local development image once:

```bash
docker build -t ayati-sandbox:dev sandbox
```

Ayati creates one persistent container from this image for each active workspace. Only that workspace is mounted at `/workspace`: read-only for Explore and read-write for Develop. `/tmp`, `/home/ayati`, and `/cache` remain writable without weakening the repository mount. The Docker socket, host home, Fireworks key, GitHub credentials, and Ayati database are not mounted.

Workspace-specific environment values are injected for each shell command through a short-lived launcher. They are not baked into this image or saved in the container configuration. Values marked for setup are also available to dependency initialization.

The image intentionally provides a small common toolset for Go, Node.js, and Python projects. Project dependencies are installed during workspace initialization and remain available until the workspace sandbox is removed.
