package purchase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
)

type SaveRequest struct{ Key, Operation, Fingerprint string }
type SaveReceipt struct {
	RequestKey   string `json:"requestKey"`
	Operation    string `json:"operation"`
	DocumentID   int64  `json:"documentId"`
	SavedVersion int64  `json:"savedVersion"`
}
type SaveResult struct {
	State    string       `json:"state"`
	Receipt  *SaveReceipt `json:"receipt,omitempty"`
	Document *Draft       `json:"document,omitempty"`
}

func prepareSaveRequest(key *string, operation string, id, version int64, h Draft, lines []Line) (*SaveRequest, error) {
	if key == nil {
		return nil, nil
	}
	k := strings.TrimSpace(*key)
	if k == "" || utf8.RuneCountInString(k) > 100 {
		return nil, ErrInvalid
	}
	// Hash validated business values without generated IDs, display data or time.
	// Keep line order; fixed-point quantities/prices normalize equivalent inputs.
	type canonicalLine struct {
		ProductID         int64
		ProductType, Unit string
		Quantity, Price   int64
		Remark            *string
	}
	items := make([]canonicalLine, len(lines))
	for i, l := range lines {
		items[i] = canonicalLine{l.ProductID, l.ProductType, l.Unit, l.QuantityMilli, l.UnitPriceCents, l.Remark}
	}
	data, e := json.Marshal(struct {
		ID, Version, PartnerID int64
		DirectDelivery         bool
		BusinessDate           BusinessDate
		Remark                 *string
		Items                  []canonicalLine
	}{id, version, h.PartnerID, h.DirectDelivery, h.BusinessDate, h.Remark, items})
	if e != nil {
		return nil, e
	}
	hash := sha256.Sum256(data)
	return &SaveRequest{k, operation, hex.EncodeToString(hash[:])}, nil
}

// Reserve before other reads/writes. A competing PostgreSQL INSERT waits on
// the unique key; SQLite serializes writers. Everything commits or rolls back
// together: reservation, draft, lines, audit and final receipt.
func (s *Service) SaveResult(ctx context.Context, actor int64, operation, key string) (SaveResult, error) {
	key = strings.TrimSpace(key)
	if actor < 1 || (operation != "CREATE" && operation != "EDIT") || key == "" || utf8.RuneCountInString(key) > 100 {
		return SaveResult{}, ErrInvalid
	}
	return s.store.SaveResult(ctx, actor, operation, key)
}
func (s *Service) ResolveSave(ctx context.Context, m audit.Metadata, operation, key string) (SaveResult, error) {
	key = strings.TrimSpace(key)
	if m.ActorID < 1 || (operation != "CREATE" && operation != "EDIT") || key == "" || utf8.RuneCountInString(key) > 100 {
		return SaveResult{}, ErrInvalid
	}
	return s.store.ResolveSave(ctx, m, operation, key)
}
