# ADR-0011: Dual-Origin KRA Gateway Architecture & Live Zero-Mock Validation

## Status

Accepted — 2026-09-21.

## Context

The original integration plan (ADR-0002 and early OSCU drafts) assumed a single uniform base URL (`KRA_OSCU_BASE_URL`) pointing to `https://sbx.kra.go.ke`. Furthermore, development environments frequently relied on mock fiscal adapters or local simulations that returned synthetic 200 OK responses to simulate eTIMS success.

During end-to-end integration and verification against live KRA infrastructure, two critical discoveries were made:

1. **Host Split between OAuth and eTIMS RPCs**:
   - The KRA API Gateway origin (`https://sbx.kra.go.ke`) only hosts the OAuth 2.0 token generation endpoint (`/v1/token/generate?grant_type=client_credentials`).
   - The actual OSCU eTIMS RPC endpoints (`/etims-api/selectInitOsdcInfo`, `/etims-api/insertTrnsSalesReq`, `/etims-api/selectItemClsCodeList`) are hosted on a separate origin: `https://etims-api-sbx.kra.go.ke` in sandbox and `https://etims-api.kra.go.ke` in production.
   - Sending eTIMS calls to `sbx.kra.go.ke` fails with 404 Not Found or HTML error pages.

2. **JSON Serialization Quirks in KRA's Gateway**:
   - KRA's OAuth token endpoint returns `"expires_in": "3599"` as a JSON **string**, rather than a standard integer. Standard Go JSON unmarshaling into `int64` failed silently, causing `token response missing access_token`.

3. **Risk of Fake Approvals in Production**:
   - Mocking or optimistic UI state transitions creates severe operational danger: a merchant might believe their tax device is initialized and compliant when KRA has actually rejected the serial or branch credentials.

## Decision

1. **Explicit Dual-Origin Configuration**:
   - Split `config.KRA` and `oscu.Config` into:
     - `BaseURL` (`KRA_OSCU_BASE_URL`): Targets OAuth token gateway (`https://sbx.kra.go.ke` or production equivalent).
     - `APIBaseURL` (`KRA_OSCU_API_BASE_URL`): Targets direct eTIMS API origin (`https://etims-api-sbx.kra.go.ke` in sandbox, `https://etims-api.kra.go.ke` in production).
   - Wired these variables into `docker-compose.yml` for both `api` and `worker` services.

2. **Tolerant JSON Parsing**:
   - Decoded `tokenResponse.ExpiresIn` as `any`, handling string, float64, and `json.Number` representations safely.

3. **Strict Zero-Mock Production Mandate & Live Handshake Verification**:
   - Defaulted `FISCAL_ADAPTER=oscu` across production and docker configurations.
   - Prohibited optimistic or simulated UI approvals. When a merchant submits Tax Settings (`POST /org/etims`), the Go backend immediately initiates a real handshake with KRA's `selectInitOsdcInfo`.
   - If an invalid device serial or mismatched TIN is provided (e.g. keyboard mashing), KRA's live rejection (such as `resultCd: "901"`, `resultMsg: "It is not valid device"`) is captured, mapped to `fiscal.ValidationError`, persisted to `orgs.etims_failed_reason`, and propagated as an HTTP 422 error. The UI displays the verbatim KRA error and remains in a failed state.

## Consequences

### Positive
- Direct, verified communication with genuine KRA eTIMS endpoints.
- Zero risk of merchants operating under false assumptions of tax compliance.
- Resilient OAuth token parsing handling upstream KRA formatting irregularities.

### Negative
- Local integration testing requires reachable KRA gateway credentials or deliberate opt-in to `FISCAL_ADAPTER=mock`.
