package httpapi

import (
	"log"
	"net/http"
)

// cancelCampaign moves a `draft`, `approved` or `sending` Campaign to
// `cancelled`, and its Deliveries not yet handed to the Provider to
// `cancelled` (ADR-0006). The worker locks the Campaign row while it claims,
// so once this returns no such Delivery is ever sent. Deliveries in flight
// finish: one the Provider rejected is cancelled, and one whose outcome is
// unknown (no answer, or its worker died) is retried with the same
// Idempotency-Key until the Provider says whether it was sent.
func (s *server) cancelCampaign(w http.ResponseWriter, r *http.Request) {
	id, ok := campaignID(w, r)
	if !ok {
		return
	}
	var cancelled bool
	var was *string
	err := s.pool.QueryRow(r.Context(), `
		WITH old AS (SELECT status FROM campaigns WHERE id = $1),
		upd AS (
			UPDATE campaigns SET status = 'cancelled'
			WHERE id = $1 AND status IN ('draft', 'approved', 'sending')
			RETURNING status
		)
		SELECT EXISTS (SELECT 1 FROM upd), (SELECT status FROM old)`, id).Scan(&cancelled, &was)
	switch {
	case err != nil:
		writeError(w, err)
	case was == nil:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": campaignNotFound})
	case !cancelled && *was == "cancelled":
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Kampanya zaten iptal edildi."})
	case !cancelled:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Tamamlanmış bir kampanya iptal edilemez."})
	default:
		// Rows a worker is claiming this instant are skipped here and
		// cancelled by the worker's next poll.
		if _, err := s.pool.Exec(r.Context(), `SELECT cancel_unsent_deliveries()`); err != nil {
			log.Printf("cancel unsent Deliveries of campaign %d: %v", id, err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "cancelled"})
	}
}
