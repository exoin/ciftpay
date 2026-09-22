-- name: CreateOrg :one
INSERT INTO orgs (name, kra_pin_enc, kra_pin_hash, vat_registered, locale)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetOrg :one
SELECT * FROM orgs WHERE id = $1;

-- name: GetOrgByPINHash :one
SELECT * FROM orgs WHERE kra_pin_hash = $1;

-- name: ListOrgs :many
SELECT * FROM orgs ORDER BY created_at DESC LIMIT $1;

-- name: UpdateOrgFiscalProfile :exec
UPDATE orgs SET fiscal_profile = $2 WHERE id = $1;

-- name: SetEtimsConfigured :one
-- The merchant's direct-OSCU device initialisation succeeded (ADR-0009):
-- persist what RegisterDevice returned and flip the progressive-onboarding
-- gate open. kra_cmc_key_enc is envelope-encrypted like kra_pin_enc, never
-- stored in clear (docs/data-model.md §4).
UPDATE orgs
SET etims_status = 'initialized', kra_bhf_id = $2, kra_device_serial = $3,
    kra_cmc_key_enc = $4, fiscal_profile = $5, etims_failed_reason = NULL, etims_initialized_at = now()
WHERE id = $1
RETURNING *;

-- name: SetEtimsFailed :one
-- RegisterDevice was rejected or unreachable; the merchant sees
-- etims_failed_reason and can fix their inputs and retry.
UPDATE orgs
SET etims_status = 'failed', etims_failed_reason = $2
WHERE id = $1
RETURNING *;

-- name: CreateUser :one
INSERT INTO users (msisdn_enc, msisdn_hash, name, locale)
VALUES ($1, $2, $3, $4)
ON CONFLICT (msisdn_hash) DO UPDATE SET name = COALESCE(NULLIF(EXCLUDED.name, ''), users.name)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByMSISDNHash :one
SELECT * FROM users WHERE msisdn_hash = $1;

-- name: CreateMembership :one
INSERT INTO memberships (org_id, user_id, role, is_default)
VALUES ($1, $2, $3, $4)
ON CONFLICT (org_id, user_id) DO UPDATE SET role = EXCLUDED.role
RETURNING *;

-- name: ListMembershipsForUser :many
SELECT m.*, o.name AS org_name
FROM memberships m JOIN orgs o ON o.id = m.org_id
WHERE m.user_id = $1
ORDER BY m.is_default DESC, o.name;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, csrf_token, expires_at, user_agent, ip)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSessionByTokenHash :one
SELECT s.*, u.name AS user_name, u.locale AS user_locale
FROM sessions s JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now();

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE id = $1;

-- name: CreateOTP :one
INSERT INTO otp_codes (msisdn_hash, code_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: LatestOTP :one
SELECT * FROM otp_codes
WHERE msisdn_hash = $1 AND consumed_at IS NULL AND expires_at > now()
ORDER BY created_at DESC LIMIT 1;

-- name: CountRecentOTPs :one
SELECT count(*) FROM otp_codes WHERE msisdn_hash = $1 AND created_at > now() - interval '1 hour';

-- name: BumpOTPAttempts :exec
UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1;

-- name: ConsumeOTP :exec
UPDATE otp_codes SET consumed_at = now() WHERE id = $1;

-- name: CreateInvite :one
INSERT INTO org_invites (org_id, invited_by, role, phone, phone_hash, email, status)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListInvitesForOrg :many
SELECT * FROM org_invites
WHERE org_id = $1 AND status = 'pending'
ORDER BY created_at DESC;

-- name: RevokeInvite :exec
UPDATE org_invites
SET status = 'revoked', updated_at = now()
WHERE id = $1 AND org_id = $2;

-- name: ListMembersForOrg :many
SELECT m.id, m.org_id, m.user_id, m.role, m.is_default, m.created_at,
       u.name AS user_name, u.msisdn_enc, u.msisdn_hash
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.org_id = $1
ORDER BY m.created_at ASC;

-- name: DeleteMembership :exec
DELETE FROM memberships
WHERE org_id = $1 AND user_id = $2;
