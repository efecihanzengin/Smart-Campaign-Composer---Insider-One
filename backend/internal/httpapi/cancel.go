package httpapi

import (
	"net/http"
	"strconv"
)

// cancelCampaign moves a `draft`, `approved` or `sending` Campaign to
// `cancelled`. The worker claims only from approved or sending Campaigns and
// locks the Campaign row while it claims, so once this returns no Delivery
// still `pending` is ever sent; Deliveries already claimed finish normally.
func (s *server) cancelCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
		return
	}
	var cancelled bool
	var was *string
	err = s.pool.QueryRow(r.Context(), `
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Kampanya bulunamadı."})
	case !cancelled && *was == "cancelled":
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Kampanya zaten iptal edildi."})
	case !cancelled:
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Tamamlanmış bir kampanya iptal edilemez."})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "status": "cancelled"})
	}
}
