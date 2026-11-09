package apiv2

import (
	"github.com/Silo-Server/silo-server/internal/newsletter"
	"github.com/Silo-Server/silo-server/internal/notifications"
)

// Native wire types keep the newsletter renderer and persistence models free
// to evolve independently of the v2 contract.
type NotificationDeliveryChoices notifications.DeliveryPreferences
type NewsletterTemplates newsletter.Templates
type NewsletterTheme newsletter.Theme
type NewsletterCustomVar newsletter.CustomVar
type NewsletterSchedule newsletter.Schedule
type NewsletterDelivery newsletter.Delivery
type NewsletterRecap newsletter.Recap
type NewsletterContent struct {
	Templates  NewsletterTemplates   `json:"templates"`
	Theme      NewsletterTheme       `json:"theme"`
	CustomVars []NewsletterCustomVar `json:"custom_vars"`
	Schedule   NewsletterSchedule    `json:"schedule"`
	Delivery   NewsletterDelivery    `json:"delivery"`
	Language   string                `json:"language"`
	Recap      NewsletterRecap       `json:"recap"`
}
type NewsletterAdminSettings struct {
	Enabled bool              `json:"enabled"`
	Content NewsletterContent `json:"content"`
}

func newsletterSettingsView(s notifications.NewsletterSettings) NewsletterAdminSettings {
	vars := make([]NewsletterCustomVar, 0, len(s.Content.CustomVars))
	for _, v := range s.Content.CustomVars {
		vars = append(vars, NewsletterCustomVar(v))
	}
	return NewsletterAdminSettings{Enabled: s.Enabled, Content: NewsletterContent{
		Templates: NewsletterTemplates(s.Content.Templates), Theme: NewsletterTheme(s.Content.Theme), CustomVars: vars,
		Schedule: NewsletterSchedule(s.Content.Schedule), Delivery: NewsletterDelivery(s.Content.Delivery),
		Language: s.Content.Language, Recap: NewsletterRecap(s.Content.Recap)}}
}
func (s NewsletterAdminSettings) domain() notifications.NewsletterSettings {
	vars := make([]newsletter.CustomVar, 0, len(s.Content.CustomVars))
	for _, v := range s.Content.CustomVars {
		vars = append(vars, newsletter.CustomVar(v))
	}
	return notifications.NewsletterSettings{Enabled: s.Enabled, Content: newsletter.Settings{
		Templates: newsletter.Templates(s.Content.Templates), Theme: newsletter.Theme(s.Content.Theme), CustomVars: vars,
		Schedule: newsletter.Schedule(s.Content.Schedule), Delivery: newsletter.Delivery(s.Content.Delivery),
		Language: s.Content.Language, Recap: newsletter.Recap(s.Content.Recap)}}
}

type NewsletterImportSummary notifications.NewsletterImportResult
type NewsletterImportSourceView struct {
	InstallationID int                      `json:"installation_id"`
	Version        string                   `json:"version"`
	Enabled        bool                     `json:"enabled"`
	Imported       bool                     `json:"imported"`
	Result         *NewsletterImportSummary `json:"result,omitempty"`
}
type NewsletterArchivedRecipientView notifications.NewsletterArchivedRecipient
type NewsletterArchivedSendView notifications.NewsletterArchivedSend
type NewsletterArchiveView struct {
	Recipients []NewsletterArchivedRecipientView `json:"recipients"`
	History    []NewsletterArchivedSendView      `json:"history"`
}

func newsletterArchiveView(s notifications.NewsletterImportArchive) NewsletterArchiveView {
	out := NewsletterArchiveView{Recipients: []NewsletterArchivedRecipientView{}, History: []NewsletterArchivedSendView{}}
	for _, r := range s.Recipients {
		out.Recipients = append(out.Recipients, NewsletterArchivedRecipientView(r))
	}
	for _, h := range s.History {
		out.History = append(out.History, NewsletterArchivedSendView(h))
	}
	return out
}
