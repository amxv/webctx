package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"sync"
	"time"
)

type localPaidReplay struct {
	fingerprint string
	output      map[string]any
	expires     time.Time
}

var paidReplayMemory = struct {
	sync.Mutex
	items map[string]localPaidReplay
}{items: map[string]localPaidReplay{}}

func localReplayKey(id string) string {
	h := sha256.Sum256([]byte(os.Getenv("FIRECRAWL_API_KEY") + "\x00" + id))
	return hex.EncodeToString(h[:])
}
func paidRequestFingerprint(calls []DataCall) string {
	body, _ := json.Marshal(calls)
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// This ledger is only a warm-worker replay optimization. It cannot claim to
// verify charges or prevent replay across multiple Vercel instances; upstream
// x-request-id idempotency remains authoritative in that situation.
func cachedPaidReplay(requestID string, calls []DataCall) (map[string]any, error) {
	key := localReplayKey(requestID)
	fingerprint := paidRequestFingerprint(calls)
	paidReplayMemory.Lock()
	entry, ok := paidReplayMemory.items[key]
	paidReplayMemory.Unlock()
	if !ok || time.Now().After(entry.expires) {
		return nil, nil
	}
	if entry.fingerprint != fingerprint {
		return nil, &DataError{
			Code:    "duplicate_request",
			Message: "This request_id already completed with different inputs. Restore the original calls to replay, or omit request_id for a new query.",
			Details: map[string]any{"recovery": "Never retry different inputs with the same ID. Use the original operation and input list for replay, or generate a fresh ID."},
		}
	}
	raw, err := json.Marshal(entry.output)
	if err != nil {
		return nil, nil
	}
	var result map[string]any
	if json.Unmarshal(raw, &result) != nil {
		return nil, nil
	}
	result["replayed"] = true
	result["replay_status"] = "served_from_local_cache"
	result["original_credits_used"] = result["credits_used"]
	result["credits_charged_this_request"] = 0
	result["billing_note"] = "This exact replay was served from Origo's in-process cache without contacting Firecrawl, so Origo incurred no new upstream execution on this attempt."
	return result, nil
}

func storePaidReplay(requestID string, calls []DataCall, output map[string]any) {
	raw, err := json.Marshal(output)
	if err != nil || len(raw) > 1<<20 {
		return
	}
	key := localReplayKey(requestID)
	paidReplayMemory.Lock()
	defer paidReplayMemory.Unlock()
	if len(paidReplayMemory.items) > 192 {
		paidReplayMemory.items = map[string]localPaidReplay{}
	}
	paidReplayMemory.items[key] = localPaidReplay{
		fingerprint: paidRequestFingerprint(calls), output: output,
		expires: time.Now().Add(15 * time.Minute),
	}
}
