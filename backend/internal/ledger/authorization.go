package ledger

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"time"

	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/storage"
)

// Shortcode statuses (mpesa_shortcodes.status, ADR-0008).
const (
	ShortcodePendingAuthorization = "pending_authorization"
	ShortcodeVerified             = "verified"
	ShortcodeRejected             = "rejected"
)

// LetterMaxBytes is the accepted size of an uploaded authorization letter.
const LetterMaxBytes = 10 << 20 // 10 MB, per api/openapi.yaml

// ErrShortcodeClaimed is returned when another organisation already holds a
// verified row for the same M-Pesa shortcode number.
var ErrShortcodeClaimed = errors.New("ledger: shortcode already verified by another organisation")

// ErrAlreadyVerified is returned by operations that only make sense before
// verification (e.g. rejecting an already-verified shortcode).
var ErrAlreadyVerified = errors.New("ledger: shortcode is already verified")

// letterContentTypes are the file types api/openapi.yaml documents as accepted.
var letterContentTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
}

// SubmitAuthorization stores the merchant's signed Safaricom authorization
// letter (ADR-0008) and marks the shortcode's paperwork as submitted. It
// never changes a `verified` row (an operator already confirmed Safaricom's
// mapping) and returns a `pending_authorization` row from `rejected`, clearing
// the previous rejection reason so the merchant's re-upload re-enters the
// operator queue.
func (s *Service) SubmitAuthorization(ctx context.Context, files storage.Store, orgID, userID, shortcodeID uuid.UUID, part *multipart.Part) (gen.MpesaShortcode, error) {
	ext, ok := letterContentTypes[part.Header.Get("Content-Type")]
	if !ok {
		return gen.MpesaShortcode{}, invalid("letter must be image/jpeg, image/png, image/webp or application/pdf")
	}
	limited := io.LimitReader(part, LetterMaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return gen.MpesaShortcode{}, err
	}
	if len(body) == 0 {
		return gen.MpesaShortcode{}, invalid("letter file is required")
	}
	if len(body) > LetterMaxBytes {
		return gen.MpesaShortcode{}, invalid("letter must be at most %d MB", LetterMaxBytes>>20)
	}

	key := fmt.Sprintf("authorizations/%s/%s-%d%s", orgID, shortcodeID, time.Now().UTC().UnixNano(), ext)

	var out gen.MpesaShortcode
	err = s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.GetShortcode(ctx, shortcodeID); err != nil {
			return err
		}
		if err := files.Put(ctx, key, bytes.NewReader(body)); err != nil {
			return err
		}
		out, err = tx.SetShortcodeAuthorizationLetter(ctx, gen.SetShortcodeAuthorizationLetterParams{ID: shortcodeID, AuthorizationLetterPath: &key})
		if err != nil {
			return err
		}
		actor := userID.String()
		return tx.AppendAudit(ctx, gen.AppendAuditParams{
			OrgID: orgID, ActorType: "user", ActorID: &actor, Action: "shortcode.authorization_submitted",
			Entity: "mpesa_shortcode", EntityID: shortcodeID.String(), After: []byte(fmt.Sprintf(`{"sha256":%q}`, sha256Hex(body))),
		})
	})
	if err != nil {
		return gen.MpesaShortcode{}, err
	}
	return out, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
