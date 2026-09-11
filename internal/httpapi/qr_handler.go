package httpapi

import (
	"net/http"
	"net/url"

	"pollservice/internal/domain"
)

func (a *API) qrPoll(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.IsUUID(id) {
		writeError(w, r, http.StatusBadRequest, "invalid_request", "invalid poll id")
		return
	}
	p, err := a.polls.GetByID(r.Context(), id)
	if err != nil {
		handleDomainError(w, r, err)
		return
	}
	if p.Status == domain.Draft {
		writeError(w, r, http.StatusConflict, "invalid_transition", "publish the poll before generating its QR code")
		return
	}
	publicURL := a.publicBaseURL + "/p/" + url.PathEscape(p.Slug)
	var svg []byte
	if cached, ok := a.qrCache.Load(publicURL); ok {
		svg = cached.([]byte)
	} else {
		if a.qr == nil {
			writeError(w, r, http.StatusServiceUnavailable, "temporarily_unavailable", "QR generator unavailable")
			return
		}
		svg, err = a.qr.GenerateSVG(publicURL)
		if err != nil {
			a.log.Error("QR generation failed", "poll_id", p.ID, "error", err)
			writeError(w, r, http.StatusServiceUnavailable, "temporarily_unavailable", "QR generation failed")
			return
		}
		a.qrCache.Store(publicURL, svg)
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `inline; filename="`+p.Slug+`.svg"`)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(svg)
}
