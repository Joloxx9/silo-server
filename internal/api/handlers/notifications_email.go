package handlers

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/Silo-Server/silo-server/internal/notifications"
)

// EmailLinkHandler serves the public tokenized email endpoints: address
// verification and unsubscribe. Both are clicked from email clients on
// devices that may have no Silo session, so they render minimal standalone
// HTML instead of redirecting into the authenticated app.
type NotificationEmailLinkProcessor interface {
	VerifyEmailToken(context.Context, string) (notifications.EmailVerifyOutcome, error)
	UnsubscribeEmail(context.Context, string) (bool, error)
}

type EmailLinkHandler struct {
	system NotificationEmailLinkProcessor
}

// NewEmailLinkHandler creates an EmailLinkHandler.
func NewEmailLinkHandler(system NotificationEmailLinkProcessor) *EmailLinkHandler {
	return &EmailLinkHandler{system: system}
}

// HandleVerify handles GET /notifications/email/verify?token=...
func (h *EmailLinkHandler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	outcome, err := h.system.VerifyEmailToken(r.Context(), r.URL.Query().Get("token"))
	switch {
	case err != nil:
		writeEmailLinkPage(w, http.StatusInternalServerError, "Something went wrong",
			"The address could not be verified. Try the link again in a moment.")
	case outcome == notifications.EmailVerifyConflict:
		writeEmailLinkPage(w, http.StatusConflict, "Address already in use",
			"This address now belongs to another profile or account. Choose a different address in Silo's notification settings.")
	case outcome == notifications.EmailVerifyInvalid:
		writeEmailLinkPage(w, http.StatusBadRequest, "Link expired or already used",
			"Request a new verification email from Silo's notification settings.")
	default:
		writeEmailLinkPage(w, http.StatusOK, "Address verified",
			"Silo notifications for this profile will now be delivered here. You can close this page.")
	}
}

// HandleUnsubscribe handles GET and POST /notifications/email/unsubscribe?token=...
// Only POST changes preferences. It is both the confirmation page's button and
// the RFC 8058 one-click target, so it needs no login and accepts no-referrer
// form posts with Origin: null. Any other request only renders the
// confirmation, so a link scanner or prefetch can't unsubscribe anyone.
// A channel=newsletter or channel=alerts link stops only that channel.
func (h *EmailLinkHandler) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	channel := r.URL.Query().Get("channel")
	t := unsubscribeTexts(r.Header.Get("Accept-Language"), channel)
	if r.Method != http.MethodPost {
		if token == "" {
			writeEmailLinkPage(w, http.StatusBadRequest, t.invalidTitle, t.invalid)
			return
		}
		writeEmailLinkPageWithConfirmation(w, http.StatusOK, t.confirmTitle, t.confirm, t.button)
		return
	}
	var ok bool
	var err error
	if byChannel, can := h.system.(interface {
		UnsubscribeEmailChannel(context.Context, string, string) (bool, error)
	}); can && channel != "" {
		ok, err = byChannel.UnsubscribeEmailChannel(r.Context(), token, channel)
	} else {
		ok, err = h.system.UnsubscribeEmail(r.Context(), token)
	}
	switch {
	case err != nil:
		writeEmailLinkPage(w, http.StatusInternalServerError, t.errorTitle, t.error)
	case !ok:
		writeEmailLinkPage(w, http.StatusBadRequest, t.invalidTitle, t.invalid)
	default:
		writeEmailLinkPage(w, http.StatusOK, t.doneTitle, t.done)
	}
}

type unsubscribeText struct {
	confirmTitle, confirm, button, doneTitle, done, invalidTitle, invalid, errorTitle, error string
}

// unsubscribeTexts picks the page's words for the browser language and the
// channel the link stops.
func unsubscribeTexts(acceptLanguage, channel string) unsubscribeText {
	polish := strings.HasPrefix(strings.ToLower(strings.TrimSpace(acceptLanguage)), "pl")
	if polish {
		t := unsubscribeText{
			confirmTitle: "Wypisać się z e-maili z powiadomieniami?",
			confirm:      "Wybierz „Wypisz się”, żeby ten profil przestał dostawać e-maile z powiadomieniami. Samo otwarcie tej strony niczego nie zmieniło.",
			button:       "Wypisz się",
			doneTitle:    "Wypisano",
			done:         "Ten profil nie będzie już dostawać e-maili z powiadomieniami. Możesz je włączyć ponownie w ustawieniach Silo.",
			invalidTitle: "Nieprawidłowy link",
			invalid:      "Ten link do wypisania się jest już nieważny. Powiadomieniami zarządzasz w ustawieniach Silo.",
			errorTitle:   "Coś poszło nie tak",
			error:        "Nie udało się wypisać. Spróbuj ponownie za chwilę.",
		}
		switch channel {
		case "newsletter":
			t.confirmTitle = "Wypisać się z newslettera?"
			t.confirm = "Wybierz „Wypisz się”, żeby ten profil przestał dostawać tygodniowy newsletter. Inne powiadomienia zostaną bez zmian. Samo otwarcie tej strony niczego nie zmieniło."
			t.done = "Ten profil nie będzie już dostawać newslettera. Możesz go włączyć ponownie w ustawieniach powiadomień w Silo."
		case "alerts":
			t.confirmTitle = "Wypisać się z powiadomień o nowościach?"
			t.confirm = "Wybierz „Wypisz się”, żeby ten profil przestał dostawać e-maile o nowościach. Newsletter zostanie bez zmian. Samo otwarcie tej strony niczego nie zmieniło."
			t.done = "Ten profil nie będzie już dostawać e-maili o nowościach. Możesz je włączyć ponownie w ustawieniach powiadomień w Silo."
		}
		return t
	}
	t := unsubscribeText{
		confirmTitle: "Unsubscribe from notification emails?",
		confirm:      "Choose Unsubscribe to stop notification emails for this profile. Opening this page has not changed your preferences.",
		button:       "Unsubscribe",
		doneTitle:    "Unsubscribed",
		done:         "This profile will no longer receive notification emails. Re-enable them any time in Silo's settings.",
		invalidTitle: "Link invalid",
		invalid:      "This unsubscribe link is no longer valid. Manage notifications in Silo's settings.",
		errorTitle:   "Something went wrong",
		error:        "Could not unsubscribe. Try the link again in a moment.",
	}
	switch channel {
	case "newsletter":
		t.confirmTitle = "Unsubscribe from the newsletter?"
		t.confirm = "Choose Unsubscribe to stop the weekly newsletter for this profile. Other notifications stay as they are. Opening this page has not changed your preferences."
		t.done = "This profile will no longer receive the newsletter. Turn it back on any time in Silo's notification settings."
	case "alerts":
		t.confirmTitle = "Unsubscribe from new-release emails?"
		t.confirm = "Choose Unsubscribe to stop new-release emails for this profile. The newsletter stays as it is. Opening this page has not changed your preferences."
		t.done = "This profile will no longer receive new-release emails. Turn them back on any time in Silo's notification settings."
	}
	return t
}

// writeEmailLinkPage renders the minimal standalone page behind tokenized
// email links.
func writeEmailLinkPage(w http.ResponseWriter, status int, title, detail string) {
	writeEmailLinkPageWithConfirmation(w, status, title, detail, "")
}

// button labels the unsubscribe form; empty renders no form.
func writeEmailLinkPageWithConfirmation(w http.ResponseWriter, status int, title, detail, button string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s — Silo</title>
<style>button{font:inherit;font-weight:600;background:#e8e8ec;color:#101014;border:0;border-radius:6px;padding:12px 20px;cursor:pointer}button:focus-visible{outline:3px solid #e8e8ec;outline-offset:4px}</style></head>
<body style="font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;background:#101014;color:#e8e8ec;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;">
<main style="max-width:420px;padding:32px;text-align:center;">
<h1 style="font-size:20px;margin:0 0 12px;">%s</h1>
<p style="font-size:14px;color:#9a9aa4;margin:0;">%s</p>
`,
		html.EscapeString(title), html.EscapeString(title), html.EscapeString(detail))
	if button != "" {
		// No action: POST back to this URL, preserving its token query and proxy
		// prefix without reflecting either into the page.
		_, _ = fmt.Fprintf(w, `<form method="post" style="margin-top:24px;"><button type="submit">%s</button></form>`, html.EscapeString(button))
	}
	_, _ = fmt.Fprint(w, `</main></body></html>`)
}
