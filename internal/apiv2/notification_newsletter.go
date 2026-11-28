package apiv2

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/Silo-Server/silo-server/internal/notifications"
	"github.com/jackc/pgx/v5"
)

type NotificationDeliveryService interface {
	DeliveryPreferences(context.Context, int, string) (notifications.DeliveryPreferences, error)
	SetDeliveryPreferences(context.Context, int, string, notifications.DeliveryPreferences) error
	NewsletterSettings(context.Context) (notifications.NewsletterSettings, error)
	SetNewsletterSettings(context.Context, notifications.NewsletterSettings) error
}

type NotificationDeliveryPreferencesOutput struct {
	Body NotificationDeliveryChoices
}
type NotificationDeliveryPreferencesInput struct {
	Body NotificationDeliveryChoices
}
type NewsletterSettingsOutput struct {
	Body NewsletterAdminSettings
}
type NewsletterSettingsInput struct {
	Body NewsletterAdminSettings
}
type NewsletterPreviewOutput struct {
	Body struct {
		Subject string `json:"subject"`
		HTML    string `json:"html"`
		Text    string `json:"text"`
	}
}
type newsletterAdminService interface {
	NewsletterRecipients(context.Context) ([]notifications.NewsletterRecipient, error)
	NewsletterTestSend(context.Context, notifications.NewsletterTest) (notifications.NewsletterTestResult, error)
	SetNewsletterRecipient(ctx context.Context, userID int, profileID, address, newsletter string) error
}
type NewsletterRecipientView struct {
	UserID       int             `json:"user_id"`
	Account      string          `json:"account"`
	ProfileID    string          `json:"profile_id"`
	ProfileName  string          `json:"profile_name"`
	IsChild      bool            `json:"is_child"`
	IsPrimary    bool            `json:"is_primary"`
	AccountEmail string          `json:"account_email" doc:"The login account's address, offered as a suggestion for the primary profile. Never used for delivery."`
	Email        string          `json:"email" doc:"Verified address; empty when the profile has none."`
	Newsletter   string          `json:"newsletter" enum:"off,daily,weekly"`
	Alerts       string          `json:"alerts" enum:"off,instant,daily"`
	LastSentAt   NullableInstant `json:"last_sent_at"`
}
type NewsletterRecipientsOutput struct {
	Body struct {
		Items []NewsletterRecipientView `json:"items"`
	}
}
type NewsletterRecipientInput struct {
	UserID    int    `path:"user_id" minimum:"1"`
	ProfileID string `path:"profile_id"`
	Body      struct {
		Email      string `json:"email" doc:"Address the admin vouches for; stored as verified. Empty removes it and stops email."`
		Newsletter string `json:"newsletter" enum:"off,weekly"`
	}
}
type NewsletterTestInput struct {
	Body struct {
		Settings  NewsletterAdminSettings `json:"settings"`
		UserID    int                     `json:"user_id,omitempty" doc:"Account of the profile whose newsletter is built; defaults to the caller."`
		ProfileID string                  `json:"profile_id,omitempty" doc:"Profile whose catalog access and history the newsletter uses; defaults to the caller's profile."`
		Mode      string                  `json:"mode" enum:"daily,weekly" doc:"weekly builds the newsletter for the last week; daily builds the daily new-release alert summary for the last day."`
		Email     string                  `json:"email,omitempty" doc:"Send the test here. Empty renders the email without sending it."`
	}
}
type NewsletterTestOutput struct {
	Body struct {
		Subject   string `json:"subject"`
		HTML      string `json:"html"`
		Text      string `json:"text"`
		ItemCount int    `json:"item_count"`
		Empty     bool   `json:"empty" doc:"Nothing arrived in the window and there is no recap to send."`
		Sent      bool   `json:"sent"`
	}
}
type NewsletterCapability struct {
	Capability
	Available   bool `json:"available"`
	Preferences bool `json:"preferences"`
}
type NewsletterCapabilityOutput struct {
	Status       int
	ETag         string `header:"ETag"`
	CacheControl string `header:"Cache-Control"`
	Body         NewsletterCapability
}

type newsletterImportService interface {
	NewsletterImportSources(context.Context) ([]notifications.NewsletterImportSource, error)
	ImportNewsletter(context.Context, int) (notifications.NewsletterImportResult, error)
	NewsletterArchive(context.Context, int) (notifications.NewsletterImportArchive, error)
}
type NewsletterArchiveInput struct {
	InstallationID int `path:"installation_id" minimum:"1"`
}
type NewsletterArchiveOutput struct {
	Body NewsletterArchiveView
}
type NewsletterImportSourcesOutput struct {
	Body struct {
		Items []NewsletterImportSourceView `json:"items"`
	}
}
type NewsletterImportInput struct {
	Body struct {
		InstallationID int `json:"installation_id" minimum:"1"`
	}
}
type NewsletterImportOutput struct {
	Body NewsletterImportSummary
}

func newsletterProblem(err error) error {
	if errors.Is(err, notifications.ErrNewsletterImport) {
		return NewProblem(TypeValidationFailed, "Disable the Email Alerts plugin and native newsletter delivery before importing. A valid installation and server encryption key are required.")
	}
	if errors.Is(err, notifications.ErrNewsletterTest) {
		return NewProblem(TypeValidationFailed, err.Error())
	}
	if errors.Is(err, notifications.ErrDeliveryPreferences) {
		return NewProblem(TypeValidationFailed, "Check the delivery settings and templates.")
	}
	if errors.Is(err, notifications.ErrEmailNoAddress) {
		return NewProblem(TypeValidationFailed, "Verify an email address before enabling email alerts or newsletters.")
	}
	if errors.Is(err, notifications.ErrEmailChildProfile) {
		return NewProblem(TypePermissionDenied, "Child profiles cannot enable email delivery.")
	}
	return serviceProblem(err)
}

func registerNotificationNewsletter(reg *Registry) {
	Register(reg, notificationOperation(http.MethodGet, "/newsletter/capability", "getNewsletterCapability"), func(ctx context.Context, _ *CapabilityInput) (*NewsletterCapabilityOutput, error) {
		out := &NewsletterCapabilityOutput{Body: NewsletterCapability{Capability: Capability{State: StateUnsupported}}}
		svc := reg.deps.NotificationDelivery
		if svc == nil {
			return out, nil
		}
		c, err := svc.NewsletterSettings(ctx)
		if err != nil {
			return nil, serviceProblem(err)
		}
		out.Body.Preferences = true
		transport, ok := svc.(interface{ NewsletterTransportAvailable(context.Context) bool })
		configured := ok && transport.NewsletterTransportAvailable(ctx)
		out.Body.Available = c.Enabled && !c.Content.Delivery.Paused && configured
		out.Body.State = configuredEnabledCapabilityState(configured, c.Enabled && !c.Content.Delivery.Paused)
		allowed := false
		if permissions, ok := svc.(interface {
			NewsletterEmailAllowed(context.Context, int, string) bool
		}); ok {
			allowed = permissions.NewsletterEmailAllowed(ctx, claimsFrom(ctx).UserID, profileFrom(ctx))
		}
		out.Body.Allowed = &allowed
		return out, nil
	})
	Register(reg, notificationOperation(http.MethodGet, "/delivery-preferences", "getNotificationDeliveryPreferences"), func(ctx context.Context, _ *struct{}) (*NotificationDeliveryPreferencesOutput, error) {
		svc := reg.deps.NotificationDelivery
		if svc == nil {
			return nil, unavailable("notification delivery")
		}
		p, err := svc.DeliveryPreferences(ctx, claimsFrom(ctx).UserID, profileFrom(ctx))
		if err != nil {
			return nil, serviceProblem(err)
		}
		return &NotificationDeliveryPreferencesOutput{Body: NotificationDeliveryChoices(p)}, nil
	})
	Register(reg, notificationOperation(http.MethodPut, "/delivery-preferences", "updateNotificationDeliveryPreferences"), func(ctx context.Context, in *NotificationDeliveryPreferencesInput) (*NotificationDeliveryPreferencesOutput, error) {
		svc := reg.deps.NotificationDelivery
		if svc == nil {
			return nil, unavailable("notification delivery")
		}
		if err := svc.SetDeliveryPreferences(ctx, claimsFrom(ctx).UserID, profileFrom(ctx), notifications.DeliveryPreferences(in.Body)); err != nil {
			return nil, newsletterProblem(err)
		}
		return &NotificationDeliveryPreferencesOutput{Body: in.Body}, nil
	})
	admin := func(method, path, id string) Operation {
		return Operation{Operation: humaOp(method, Prefix+"/admin/notifications/newsletter"+path, id, "admin", "Configure built-in newsletters and new-media email alerts."), Class: ClassActingAdmin, DemoRestricted: method != http.MethodGet, ServiceBacked: true}
	}
	Register(reg, admin(http.MethodGet, "/import", "getNewsletterImportSources"), func(ctx context.Context, _ *struct{}) (*NewsletterImportSourcesOutput, error) {
		svc, ok := reg.deps.NotificationDelivery.(newsletterImportService)
		if !ok {
			return nil, unavailable("newsletter import")
		}
		items, err := svc.NewsletterImportSources(ctx)
		if err != nil {
			return nil, serviceProblem(err)
		}
		out := &NewsletterImportSourcesOutput{}
		out.Body.Items = []NewsletterImportSourceView{}
		for _, source := range items {
			var result *NewsletterImportSummary
			if source.Result != nil {
				value := NewsletterImportSummary(*source.Result)
				result = &value
			}
			out.Body.Items = append(out.Body.Items, NewsletterImportSourceView{InstallationID: source.InstallationID, Version: source.Version, Enabled: source.Enabled, Imported: source.Imported, Result: result})
		}
		return out, nil
	})
	importOp := admin(http.MethodPost, "/import", "importNewsletterPlugin")
	importOp.RetrySafety = RetrySafetyNaturalIdempotent
	Register(reg, importOp, func(ctx context.Context, in *NewsletterImportInput) (*NewsletterImportOutput, error) {
		svc, ok := reg.deps.NotificationDelivery.(newsletterImportService)
		if !ok {
			return nil, unavailable("newsletter import")
		}
		result, err := svc.ImportNewsletter(ctx, in.Body.InstallationID)
		if err != nil {
			return nil, newsletterProblem(err)
		}
		return &NewsletterImportOutput{Body: NewsletterImportSummary(result)}, nil
	})
	Register(reg, admin(http.MethodGet, "/import/{installation_id}", "getNewsletterImportArchive"), func(ctx context.Context, in *NewsletterArchiveInput) (*NewsletterArchiveOutput, error) {
		svc, ok := reg.deps.NotificationDelivery.(newsletterImportService)
		if !ok {
			return nil, unavailable("newsletter import")
		}
		archive, err := svc.NewsletterArchive(ctx, in.InstallationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, NewProblem(TypeNotFound, "Newsletter import not found.")
		}
		if err != nil {
			return nil, serviceProblem(err)
		}
		return &NewsletterArchiveOutput{Body: newsletterArchiveView(archive)}, nil
	})
	Register(reg, admin(http.MethodGet, "", "getNewsletterSettings"), func(ctx context.Context, _ *struct{}) (*NewsletterSettingsOutput, error) {
		svc := reg.deps.NotificationDelivery
		if svc == nil {
			return nil, unavailable("newsletter")
		}
		c, err := svc.NewsletterSettings(ctx)
		if err != nil {
			return nil, serviceProblem(err)
		}
		return &NewsletterSettingsOutput{Body: newsletterSettingsView(c)}, nil
	})
	update := admin(http.MethodPut, "", "updateNewsletterSettings")
	update.RetrySafety = RetrySafetyNaturalIdempotent
	Register(reg, update, func(ctx context.Context, in *NewsletterSettingsInput) (*NewsletterSettingsOutput, error) {
		svc := reg.deps.NotificationDelivery
		if svc == nil {
			return nil, unavailable("newsletter")
		}
		if err := svc.SetNewsletterSettings(ctx, in.Body.domain()); err != nil {
			return nil, newsletterProblem(err)
		}
		c, err := svc.NewsletterSettings(ctx)
		if err != nil {
			return nil, serviceProblem(err)
		}
		return &NewsletterSettingsOutput{Body: newsletterSettingsView(c)}, nil
	})
	preview := admin(http.MethodPost, "/preview", "previewNewsletter")
	preview.RetrySafety = RetrySafetyNaturalIdempotent
	Register(reg, preview, func(ctx context.Context, in *NewsletterSettingsInput) (*NewsletterPreviewOutput, error) {
		content := in.Body.domain().Content
		content.Normalize()
		r, err := newsletter.Preview(ctx, content)
		if err != nil {
			return nil, NewProblem(TypeValidationFailed, "The newsletter template could not be rendered.")
		}
		out := &NewsletterPreviewOutput{}
		out.Body.Subject = r.Subject
		out.Body.HTML = r.HTML
		out.Body.Text = r.Text
		return out, nil
	})
	Register(reg, admin(http.MethodGet, "/recipients", "getNewsletterRecipients"), func(ctx context.Context, _ *struct{}) (*NewsletterRecipientsOutput, error) {
		svc, ok := reg.deps.NotificationDelivery.(newsletterAdminService)
		if !ok {
			return nil, unavailable("newsletter")
		}
		items, err := svc.NewsletterRecipients(ctx)
		if err != nil {
			return nil, serviceProblem(err)
		}
		out := &NewsletterRecipientsOutput{}
		out.Body.Items = make([]NewsletterRecipientView, 0, len(items))
		for _, r := range items {
			out.Body.Items = append(out.Body.Items, NewsletterRecipientView{UserID: r.UserID, Account: r.Account, ProfileID: r.ProfileID,
				ProfileName: r.ProfileName, IsChild: r.IsChild, IsPrimary: r.IsPrimary, AccountEmail: r.AccountEmail, Email: r.Email, Newsletter: r.Newsletter, Alerts: r.Alerts, LastSentAt: nullableInstantOf(r.LastSentAt)})
		}
		return out, nil
	})
	setRecipient := admin(http.MethodPut, "/recipients/{user_id}/{profile_id}", "setNewsletterRecipient")
	setRecipient.RetrySafety = RetrySafetyNaturalIdempotent
	setRecipient.DefaultStatus = http.StatusNoContent
	Register(reg, setRecipient, func(ctx context.Context, in *NewsletterRecipientInput) (*struct{}, error) {
		svc, ok := reg.deps.NotificationDelivery.(newsletterAdminService)
		if !ok {
			return nil, unavailable("newsletter")
		}
		b := in.Body
		if b.Email == "" {
			b.Newsletter = "off"
		}
		if err := svc.SetNewsletterRecipient(ctx, in.UserID, in.ProfileID, strings.TrimSpace(b.Email), b.Newsletter); err != nil {
			return nil, newsletterProblem(err)
		}
		return nil, nil
	})
	// Each call sends another email, so a client must not repeat it blindly.
	testOp := admin(http.MethodPost, "/test", "testNewsletter")
	testOp.RetrySafety = RetrySafetyNonRetryable
	Register(reg, testOp, func(ctx context.Context, in *NewsletterTestInput) (*NewsletterTestOutput, error) {
		svc, ok := reg.deps.NotificationDelivery.(newsletterAdminService)
		if !ok {
			return nil, unavailable("newsletter")
		}
		user, profile := in.Body.UserID, in.Body.ProfileID
		if profile == "" {
			user, profile = claimsFrom(ctx).UserID, profileFrom(ctx)
		}
		result, err := svc.NewsletterTestSend(ctx, notifications.NewsletterTest{Settings: in.Body.Settings.domain().Content,
			UserID: user, ProfileID: profile, Mode: in.Body.Mode, To: in.Body.Email})
		if err != nil {
			var tmpl *newsletter.TemplateError
			if errors.As(err, &tmpl) {
				return nil, NewProblem(TypeValidationFailed, "The newsletter template could not be rendered.")
			}
			return nil, newsletterProblem(err)
		}
		out := &NewsletterTestOutput{}
		out.Body.ItemCount, out.Body.Sent, out.Body.Empty = result.ItemCount, result.Sent, result.Rendered == nil
		if r := result.Rendered; r != nil {
			out.Body.Subject, out.Body.HTML, out.Body.Text = r.Subject, r.HTML, r.Text
		}
		return out, nil
	})
}
