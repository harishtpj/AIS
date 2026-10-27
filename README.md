# AIS

API Integration Suite — a Go API gateway with a Ruby configuration DSL.

## Static services and backend login paths

A service's client-facing AIS login endpoint remains `POST /auth/<service>/login`. The backend login endpoint is configurable per service through `auth_path`; it does not have to be `/login`.

```ruby
AIS.configure do
  unified_auth true

  service :orders, 'http://localhost:9001' do
    auth :sessions
    auth_path '/login'
  end

  service :inventory, 'http://localhost:9002' do
    auth :opq_tok
    auth_path '/api/v1/token'
  end

  route '/orders', :orders
  route '/inv', :inventory
  bind '0.0.0.0:8080'
end
```

The older options-hash form remains supported:

```ruby
service :orders, 'http://localhost:9001', auth: :sessions, auth_path: '/login'
```

## Podman dynamic discovery

Discovery is opt-in, so existing static-only deployments continue to work. Enable it in `.env`:

```dotenv
AIS_DISCOVERY=podman
AIS_DISCOVERY_INTERVAL=5
AIS_PODMAN_SOCKET=/run/user/1000/podman/podman.sock
AIS_PODMAN_HOST=127.0.0.1
```

Use the correct UID in the socket path. The default socket is derived from `XDG_RUNTIME_DIR` when `AIS_PODMAN_SOCKET` is omitted. The Podman user socket must be enabled, for example:

```bash
systemctl --user enable --now podman.socket
```

Label each Podman pod in its Kubernetes YAML manifest. AIS uses the `ais/<label>` namespace:

```yaml
metadata:
  labels:
    ais/discover: "true"
    ais/service: "orders"
    ais/auth-strategy: "sessions"
    ais/auth-path: "/login"
    ais/port: "8080"
```

Supported labels:

- `ais/discover`: must be `"true"` to opt the pod in.
- `ais/service`: logical service name referenced by `route`.
- `ais/port`: container port to look up in the pod's published port bindings.
- `ais/auth-strategy`: `sessions`, `session`, `opq_tok`, `opaque-token`, or `none` (omitted means no unified auth).
- `ais/auth-path`: backend login path; defaults to `/login`.

The provider uses the official `go.podman.io/podman/v6/pkg/bindings` Go library over the Podman Unix socket. It reads pod labels and published port bindings, then builds host-reachable URLs using `AIS_PODMAN_HOST` plus the published host port. If multiple pods share a logical service name and use identical auth metadata, AIS rotates requests across their endpoints. Manual `service` declarations take precedence over discovered services with the same name.

Routes may reference a discovered service even when it is not present during configuration loading:

```ruby
route '/orders', :orders
```

If the backend has not been discovered yet, requests to that route return `502 Service Unavailable` until a successful discovery cycle registers it. A failed Podman API poll preserves the last successful registry snapshot; a successful poll removes instances that have disappeared. Discovery runs every five seconds by default, configurable with `AIS_DISCOVERY_INTERVAL` (seconds).

Only enable discovery against a trusted Podman socket. Access to that socket grants substantial control over containers on the host. Pod labels are metadata, not a security boundary.

## Updating Go dependencies

After applying this patch, run `go mod tidy` with the Go version declared by this project. This resolves the Podman bindings module and updates `go.sum`.
