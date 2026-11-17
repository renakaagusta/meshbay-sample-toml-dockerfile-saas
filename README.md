# meshbay-sample-toml-dockerfile-saas

Dockerfile variant of [meshbay-sample-saas](https://github.com/renakaagusta/meshbay-sample-saas): same three
services, two databases and one repository, but every service builds from a Dockerfile instead of Railpack.

| service | Dockerfile | how it builds |
|---|---|---|
| `web` | `web/Dockerfile` | `node:22.21.1-alpine3.22` installs the root dependencies and runs `next build web`; the runtime stage reuses that `node_modules` (the app's `next.config.ts` needs `typescript` at startup) and starts `npm run start` on port 3000 |
| `api` | `api/Dockerfile` | unchanged from the original: `golang:1.23-alpine` builds the server and the migrate binary, `alpine:3.20` runs them as a non-root user |
| `worker` | `worker/Dockerfile` | `node:22.21.1-alpine3.22` with production dependencies only, starts `node worker/src/worker.js` |

The build context is still the repository root (Meshbay always builds from the root), so every `COPY` path in a
Dockerfile is root-relative and each Dockerfile sits next to the service it builds.

Meshbay fixture (MESH-132): **one repository, three services and two databases**. It exercises what the
single-app samples never touch — several services from one repo, the private service network, SQL
migrations in `pre_deploy`, `replicas = 2` spread over the cluster, a Redis queue consumed by a separate
worker, a sealed variable, and a cron command.

- `web` — Next.js 15 on Node 22, lists items and posts new ones. Reads `API_URL` **at runtime**, server side.
- `api` — Go 1.23, `net/http` + `pgx` + `go-redis`. Item CRUD with a `status` column (`queued` → `processed`),
  `/api/stats`, and an `X-Signature` HMAC header signed with the sealed `SIGNING_SECRET`.
- `worker` — Node 22 consumer of the Redis list; marks each item processed through the api on the private
  network and logs one line per job. `node worker/src/daily-summary.js` is the cron command.
- Postgres — items, statuses, `schema_migrations`.
- Redis — the `jobs` list shared by api (LPUSH) and worker (BRPOP).

```
                          public HTTPS
   browser ────────────────────┬───────────────────────────────────┐
                               ▼                                   │
                    ┌─────────────────────────┐                    │
                    │ web   (Next.js 15)       │                    │
                    │ API_URL read at runtime  │                    │
                    └───────────┬─────────────┘                    │
                                │ http://<project>-api.internal:8080   │
                    ┌───────────▼─────────────┐        ┌───────────▼──────────┐
                    │ api  ×2  (Go 1.23)      ├───────►│ Postgres             │
                    │ CRUD · /api/stats       │        │ items · schema_      │
                    │ X-Signature (HMAC)      │        │ migrations           │
                    └───────────┬─────────────┘        └──────────────────────┘
                                │ LPUSH jobs
                    ┌───────────▼─────────────┐
                    │ Redis (Meshbay database)│◄──── BRPOP jobs ────┐
                    └─────────────────────────┘                     │
                                                     ┌──────────────▼─────────┐
   cron 07:00 ── node worker/src/daily-summary.js ──►│ worker (Node 22)       │
                                                     │ POST /api/items/:id/   │
                                                     │ process → processed    │
                                                     └────────────────────────┘
```

## Repository layout

```
meshbay-sample-toml-dockerfile-saas
├── meshbay.toml            web + api + worker, ports, replicas, healthchecks, pre_deploy
├── package.json            one node_modules for web and worker (repo root is the build context)
├── web/                    Next.js 15 app router: page, form, proxy routes, /health
│   └── Dockerfile          npm ci + next build web, runtime starts npm run start on 3000
├── worker/src/             worker.js (queue consumer), daily-summary.js (cron), api-url.js (address resolution)
├── worker/Dockerfile       production dependencies, starts node worker/src/worker.js
├── api/                    Go module: cmd/server, cmd/migrate, internal/app, internal/migrate
│   └── Dockerfile          builds /app/server and /app/migrate
├── .dockerignore           keeps .git, node_modules, .next and .env out of every build context
└── scripts/smoke.sh        end-to-end check, prints OK
```

## Environment

| Variable | Used by | Required | Default | Description |
|---|---|---|---|---|
| `DATABASE_URL` | api, migrate | yes | none | Postgres URL, e.g. `postgres://user:password@host:5432/app` |
| `REDIS_URL` | api, worker | yes | none | Redis URL, e.g. `redis://host:6379/0` |
| `API_URL` | web, worker | no | `http://$MESHBAY_PROJECT-api.internal:8080`, else `http://localhost:8080` | api base URL. Set it as a project variable to override the derived private address |
| `MESHBAY_PROJECT` | web, worker | no | injected | Project slug, injected by Meshbay into every manifest service. A service on the private network is `<project>-<service>.internal`, so the api is `<project>-api.internal` |
| `SIGNING_SECRET` | api | no | empty | **sealed** secret. When set, every response carries `X-Signature: sha256=<hmac>` |
| `PORT` | web, api | no | `3000` / `8080` | Injected by Meshbay from the container port in `meshbay.toml` |
| `QUEUE_NAME` | worker | no | `jobs` | Redis list name |

`meshbay.toml` declares the variable *names* with empty values; the real values come from the project
variables and, at deploy time, project variables win over the manifest (`mergedEnvironment` in
`apps/api/internal/application/runner/execute.go`).

`API_URL` is deliberately empty in the manifest. Meshbay cannot interpolate the project slug into a manifest
value (gap 7 below), so `web/lib/api.ts` and `worker/src/api-url.js` build the address at runtime from
`MESHBAY_PROJECT` instead of hard-coding a host name that would be wrong for every project slug.
`MESHBAY_PRIVATE_DOMAIN` is the other injected name: it is the service's *own* domain (`<project>-web.internal`).

## Endpoints

Served by `api` (and proxied by `web` under the same paths, so the browser never sees `API_URL`):

| Method | Path | Description |
|---|---|---|
| GET | `/health` | `200` with `version`, `database`, `redis`; `503` when a dependency is unreachable |
| GET | `/api/items` | JSON array of items, newest last |
| POST | `/api/items` | `{"name":"..."}` → `201` item with `status:"queued"`, job pushed to Redis |
| GET | `/api/items/{id}` | `200` item, `404` when missing |
| PUT | `/api/items/{id}` | `{"name":"..."}` → `200` renamed item, `404` when missing |
| DELETE | `/api/items/{id}` | `204`, `404` when missing |
| POST | `/api/items/{id}/process` | worker callback: `queued` → `processed`, idempotent |
| GET | `/api/stats` | `{"queued":n,"processed":n,"total":n}` |

Served by `web`:

| Method | Path | Description |
|---|---|---|
| GET | `/` | list + stats + form |
| GET | `/health` | probes `api` `/health`; `502` when the api is unreachable |
| GET, POST | `/api/items` | proxy to the api |
| GET, PUT, DELETE | `/api/items/{id}` | proxy to the api |
| GET | `/api/stats` | proxy to the api |

## Test scenario (step by step)

1. Create a project in Meshbay from `https://github.com/renakaagusta/meshbay-sample-toml-dockerfile-saas`, branch `main`.
2. Add a **Postgres** database to the project and attach it. Meshbay provisions `DATABASE_URL`.
3. Add a **Redis** database to the project and attach it. Meshbay provisions `REDIS_URL`.
4. Add the project variable `SIGNING_SECRET` (sealed, any random string).
5. Deploy. Expected services: `web` (1 replica, public domain), `api` (2 replicas, public
   `<project>-api` domain, `pre_deploy` runs `/app/migrate`), `worker` (1 replica, no domain).
6. Check the api log for `applied migration 0001_items.sql` and `listening on port 8080` twice (two replicas).
7. Check the worker log for `starting api=http://<project>-api.internal:8080 queue=jobs`.
8. Run the smoke script against both public URLs:

   ```sh
   bash scripts/smoke.sh https://<project>.renakaagusta.dev https://<project>-api.renakaagusta.dev
   ```

   Only the web URL is required: `bash scripts/smoke.sh https://<project>.renakaagusta.dev`.
   The api URL adds the direct probes (dependency health, `X-Signature`, direct CRUD).
9. Open the web page, add an item, watch the status go `queued` → `processed` after the worker picks it up.
10. Register the cron job in Meshbay against the `worker` service and run it once by hand:

    | Field | Value |
    |---|---|
    | name | `daily-summary` |
    | schedule | `0 7 * * *` |
    | service | `worker` |
    | command | `node worker/src/daily-summary.js` |

    The run log should end with
    `daily-summary at=... total=... queued=... processed=... oldest_queued_id=...`.

The smoke script asserts, in order: web health → api health, Postgres, Redis and `X-Signature` → item created
through the web app is `queued` → the worker flips it to `processed` within 60 s → `/api/stats` counts it →
full CRUD → prints `OK`.

## Run locally

```sh
# 1. Postgres and Redis: any local pair works
export DATABASE_URL=postgres://postgres:postgres@localhost:5432/app
export REDIS_URL=redis://localhost:6379/0
export SIGNING_SECRET=local-secret
export API_URL=http://localhost:8080

# 2. Migrations, exactly like pre_deploy does
(cd api && go run ./cmd/migrate)

# 3. api
(cd api && go run ./cmd/server)

# 4. web (second shell)
npm install
npm run build
PORT=3000 npm start

# 5. worker (third shell)
npm run worker

# 6. the cron command, once
npm run summary
```

## Checks run before pushing

```sh
~/bin/buildslot go vet ./...                # in api/
~/bin/buildslot go build -o /dev/null ./... # in api/
~/bin/buildslot npm ci                      # repository root
~/bin/buildslot npm run check               # tsc --noEmit for web + node --check for worker
bash -n scripts/smoke.sh
shellcheck scripts/smoke.sh
```

Every image was built locally from the repository root:

```sh
~/bin/buildslot docker build -f api/Dockerfile -t sample-saas-api .
~/bin/buildslot docker build -f web/Dockerfile -t sample-saas-web .
~/bin/buildslot docker build -f worker/Dockerfile -t sample-saas-worker .
```

## Known Meshbay gaps

Found while writing this fixture. None of them is fixed in Meshbay; where the fixture itself has to sidestep a
gap to boot, the entry says how. Each one is reported as its own ticket.

1. **One repository builds one service (fixed).** When this fixture was written, build detection read only the
   first service of `meshbay.toml` (`manifestPlan(parsed.Services[0])` in
   `apps/api/internal/application/builddetect/manifestplan.go`), while the deploy gave every declared service an
   image name of `<project>-<service>:<sha>` (`imagesFor` in `apps/api/internal/application/autodeploy/plan.go`),
   so `api` and `worker` had no build behind their images. Main now plans every service (`manifestPlans`) and
   `imagesFor` maps each one to the image built for it; this Dockerfile variant relies on that, since `web`, `api`
   and `worker` each build from their own Dockerfile.
2. **No per-service root directory.** The build context is always the repository root and a Dockerfile path is
   relative to it, so every command carries its folder (`next build web`, `node worker/src/worker.js`, and
   `COPY api/...` in the Dockerfile). A service cannot say "this repo, but build `api/`".
3. **Cron jobs cannot be declared in `meshbay.toml`.** A cron job is a project record (`domain.CronJob`:
   name, schedule, command, service, image), created in the UI. Step 10 above has the values for this repo.
4. **Per-service variables versus manifest services.** Variables are resolved against the cluster service name
   (`<project>-<service>`, see `ClusterServiceName` in `apps/api/internal/domain/environment.go`), while the
   source a user creates is named `api`/`web`. `variables.Resolver.scope` translates a source name to a cluster
   name but not the other way round, so a per-service variable may never reach a service declared in
   `meshbay.toml`; shared project variables always do. This repo derives the api address from
   `MESHBAY_PROJECT` (an injected, not a stored, variable) and expects the databases'
   `DATABASE_URL`/`REDIS_URL` to be shared variables.
5. **`pre_deploy` semantics are unverified.** Whether a hook runs once per deploy or once per replica is not
   documented (open question in MESH-132). The migrations are idempotent (`schema_migrations` + `IF NOT EXISTS`),
   so two replicas running them concurrently must not fail — worth watching in the deploy log.
6. **Worker healthcheck.** Meshbay's command healthcheck needs a binary that exists in the image; the Node slim
   image has no `pgrep`, so the worker has no healthcheck rather than a fake one. A container-level "is the
   process alive" check would be a useful Meshbay default.
7. **No interpolation in manifest values.** A service address is `<project>-<service>.internal`
   (`ClusterServiceName` + `PrivateDomain` in `apps/api/internal/domain`), but `meshbay.toml` cannot reference the
   project slug: env values are copied verbatim, and only `MESHBAY_PROJECT` is injected at deploy time. So
   `API_URL` cannot be written in the manifest at all and has to be derived in code or set by hand as a project
   variable. A `${project}` / `{{project}}` placeholder resolved at plan time
   (`DeploymentSpecs` in `apps/api/internal/application/manifest/deploy.go`) would remove that workaround.
