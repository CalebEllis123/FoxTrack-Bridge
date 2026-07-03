# Docker

FoxTrack Bridge can run in a container using the headless Linux build. The container stores settings and print history under `/data`.

## Build locally

From the repository root:

```bash
docker build \
  --build-arg APP_VERSION=v2.1.4 \
  -t foxtrack-bridge .
```

Use the latest FoxTrack Bridge release tag for `APP_VERSION` when building images for regular server use. The update checker compares this value with the latest GitHub release; branch names, commit hashes, or other non-release values may be reported as older than the current release.

For development builds, pass a unique version string if you want the dashboard to show exactly which image is running:

```bash
docker build \
  --build-arg APP_VERSION="$(git describe --tags --always --dirty)" \
  -t foxtrack-bridge .
```

## Run

```bash
docker run -d \
  --name foxtrack-bridge \
  -p 8080:8080 \
  -v foxtrack-bridge-data:/data \
  --restart unless-stopped \
  foxtrack-bridge
```

Open the dashboard at [http://localhost:8080](http://localhost:8080).

## Build from a fork or branch

Docker can build directly from a GitHub repository:

```bash
docker build \
  --build-arg APP_VERSION=v2.1.4 \
  -t foxtrack-bridge \
  https://github.com/jonathanq/FoxTrack-Bridge.git#add-docker-support
```

Replace the repository, branch, and `APP_VERSION` value for your own fork or release.

## Notes

- The image runs as a non-root `foxtrack` user.
- Persist `/data` to keep settings and print history across container updates.
- The app listens on port `8080` inside the container.
- In container deployments, update by rebuilding or pulling a new image and recreating the container. The in-app updater is intended for native binary installs.
