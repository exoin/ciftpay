# ADR-0013: Self-Healing Onboarding and KRA PIN Lockout Recovery

## Status

Accepted — 2026-09-20.

## Context

In the initial implementation of tenant onboarding, a strict uniqueness constraint was enforced on `orgs.kra_pin_enc` / taxpayer PIN lookup during the earliest step of the onboarding funnel.

During testing and real user trials, a major onboarding bug emerged:
- If a prospective merchant started onboarding, input their genuine KRA PIN, but abandoned the process before completing shortcode verification or account setup (e.g., closed the tab, had a network disconnection, or navigated away), their KRA PIN remained bound to an incomplete, unverified organization.
- If the merchant returned later to sign up again, or if another user within the business attempted registration, the system rejected the registration with `409 Conflict: PIN already registered`. The genuine owner was permanently locked out and unable to complete setup without manual database surgery by an engineer.

## Decision

1. **State-Aware PIN Uniqueness**:
   - Differentiated between *active/verified* merchant organizations and *abandoned/unverified* onboarding sessions.
   - If an unverified organization with the same PIN is abandoned prior to shortcode/business verification, the onboarding flow permits the genuine owner (authenticated via phone OTP) to reclaim and resume their existing organization or overwrite the stale unverified state.
   - Only organizations that have achieved active status or verified a shortcode enforce absolute global lockouts on their KRA PIN.

2. **Differentiated Auth UX**:
   - Redesigned the authentication screen to clearly distinguish between "Sign In" (existing merchants returning to their dashboard) and "Sign Up" (new businesses onboarding for the first time), preventing users from accidentally attempting duplicate registration.

## Consequences

### Positive
- Prevents customer drop-off and support escalations from abandoned sign-up attempts.
- Legitimate business owners can resume onboarding seamlessly.

### Negative
- Requires careful session validation to prevent unauthorized hijacking of unverified PINs.
