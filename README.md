# ☁️ AIS - API Integration Suite

API Integration Suite (AIS) is an API gateway written in Go. It acts as an
interoperability layer between client applications and heterogeneous backend
services. AIS provides reverse proxying, unified authentication, programmable
request policies, service discovery, and the foundation for semantic API
translation.

# 🌟 Features

*   Reverse proxying with configurable route prefixes
*   Longest-prefix route matching
*   Unified client authentication using JWT
*   Backend credential translation for session cookies and opaque bearer tokens
*   Programmable and sandboxed policy engine using Ruby
*   Dynamic policy-based routing
*   Request-aware policies using method, path, client IP, time, and location
*   Canonical API entities stored in a versioned Redis ontology database
*   Deterministic nested JSON field translation
*   Automatic service discovery using Podman
*   Multiple backend instances for a single logical service
*   Round-robin instance selection
*   Configurable backend authentication paths
*   Structured logging with Zerolog
*   Graceful shutdown and configurable server timeouts

Among these, the **core** features of the gateway includes:

##### Semantic API Translation

Different services may represent the same business resource using different
field names and JSON structures. AIS is designed to expose a canonical API to
clients and translate it into the format expected by each backend.

For example, a canonical request may contain:

```json
{
  "item": {
    "name": "Laptop",
    "price": 1200
  }
}
```

But a particular backend service coud expect the payload in a different way. Hence, the same 
request can be translated as:

```json
{
  "details": {
    "title": "Laptop"
  },
  "pricing": {
    "amount": 1200
  }
}
```

AIS stores canonical entities as versioned documents in Redis. The translation
engine supports explicit nested field mappings such as:

```text
item.name  -> details.title
item.price -> pricing.amount
```

The ontology and deterministic translation layers are available as the
foundation for backend schema ingestion and automatic mapping inference.

### Unified Authentication

Clients authenticate with AIS using a single JWT-based interface:

```text
POST /auth/<service>/login
```

AIS forwards the login request to the configured backend, stores the backend
credential in Redis, and returns an AIS JWT to the client. During subsequent
requests, AIS validates the JWT and translates it into the credential required
by the selected backend.

The currently supported backend authentication strategies are:

*   `sessions` - backend session cookies
*   `opq_tok` - opaque bearer tokens

The AIS JWT is not forwarded to the backend. Backends receive only their
original credential format.

### Programmable Policy Engine

AIS embeds a sandboxed Ruby policy engine. Administrators can define policies
without changing the Go gateway implementation.

Policies can use:

*   HTTP method and request path
*   Client IP address
*   Current time, date and geolocation
*   Service and route constraints
*   Dynamic backend selection

A sample configuration file would be like:

```ruby
AIS.configure do
  unified_auth false

  service :orders, 'http://localhost:9001'
  service :inventory, 'http://localhost:9002'

  route '/items', :inventory

  policy 'Route items by time' do
    for_services :orders, :inventory
    for_routes '/items'

    evaluate do |request|
      if request.timeinfo.now.hour < 12
        route_to :inventory
      else
        route_to :orders
      end
    end
  end

  bind '0.0.0.0:8080'
end
```

Policies are restricted to services declared in their `for_services` list.
This prevents a policy from routing requests to undeclared backends.

### Service Discovery

Currently, service discovery is opt-in and supports backend with Podman. 
A running Podman pod can register itself with AIS using labels:

```yaml
metadata:
  labels:
    ais/discover: "true"
    ais/service: "orders"
    ais/auth-strategy: "sessions"
    ais/auth-path: "/login"
    ais/port: "8080"
```

AIS groups pods with the same logical service name and rotates requests across
their published endpoints. Static service declarations take precedence over
discovered services.

Enable discovery using:

```dotenv
AIS_DISCOVERY=podman
AIS_DISCOVERY_INTERVAL=1000
AIS_PODMAN_SOCKET=/run/user/1000/podman/podman.sock
AIS_PODMAN_HOST=127.0.0.1
```

The Podman socket must be trusted because access to it provides control over
containers on the host.

# ⚙️ Building

AIS requires Go 1.27 or later.

Clone and build the gateway:

```bash
$ git clone --recurse-submodules https://github.com/harishtpj/AIS.git
$ cd AIS
$ make
```

This project uses MRuby implementation of C for embedding Ruby into the gateway. Support for
mruby bindings are derived via [gruby](https://github.com/zhulik/gruby/)

# 🚀 Usage

Start the Redis DB and insert the credentials onto the `.env` file. Alternatively, you may
start the podman container for redis:

```bash
$ podman play kube redis.yml
```

Start AIS with the default configuration:

```bash
$ ./ais start --config configs/ais.rb
```

The gateway listens on the address specified by the `bind` directive. The
default example configuration uses `0.0.0.0:8080`

You can also start with a canonical entity file:

```bash
$ ./ais start \
  --config configs/ais.rb \
  --canonical examples/canonical-api.yaml
```

The canonical entity YAML contains reusable resource definitions:

```yaml
version: v1
entities:
  - name: Item
    fields:
      id:
        type: string
        identity: true
      name:
        type: string
        required: true
      price:
        type: number
        required: true
```

The canonical entity is validated and persisted in Redis during startup.
The Redis key layout is documented in [`docs/ontology-db.md`](docs/ontology-db.md).

## Configuration DSL

The Ruby configuration file supports:

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
  route '/inventory', :inventory
  bind '0.0.0.0:8080'
end
```

The service options are:

*   `auth :sessions`
*   `auth :opq_tok`
*   `auth :none`
*   `auth_path '/path'`

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `AIS_JWT_SECRET` | Secret used to sign AIS JWTs | Required for unified auth |
| `REDIS_ADDR` | Redis address for authentication sessions | `127.0.0.1:6379` |
| `REDIS_PASS` | Redis password | Empty |
| `REDIS_DB` | Redis database for authentication sessions | `0` |
| `AIS_ONTOLOGY_REDIS_ADDR` | Redis address for ontology data | `REDIS_ADDR` |
| `AIS_ONTOLOGY_REDIS_PASS` | Redis password for ontology data | `REDIS_PASS` |
| `AIS_ONTOLOGY_REDIS_DB` | Redis database for ontology data | `REDIS_DB` |
| `AIS_DISCOVERY` | Service discovery mode | Disabled |
| `AIS_DISCOVERY_INTERVAL` | Discovery interval in seconds | `5` |
| `AIS_PODMAN_SOCKET` | Podman Unix socket path | Derived from `XDG_RUNTIME_DIR` |
| `AIS_PODMAN_HOST` | Host used for discovered endpoints | `127.0.0.1` |

# 🛣️ Roadmap

The following components are part of the planned semantic integration layer:

*   OpenAPI schema ingestion using a Go OpenAPI parser
*   Automatic backend schema registration during service discovery
*   Ontology-assisted field matching
*   Optional local SLM inference through an Ollama sidecar
*   Versioned request and response translation plans
*   Role-aware policy evaluation
*   Rate limiting, quotas, and failover policies
*   Health checks and expanded observability using OpenTelemetry and Prometheus

The SLM is intended for schema analysis and ambiguous mapping suggestions only.
It should not be called on the request path. Production requests should use
compiled deterministic translation plans.

# 📝 License

#### Copyright © 2026 [M.V.Harish Kumar](https://github.com/harishtpj).

#### This project is [MIT](LICENSE) licensed.
