package main

import (
	"testing"
)

func TestTrapBansHumansAndOtherBotsButExemptsThisBot(t *testing.T) {
	for _, user := range []string{"user", "mod", "otherbot"} {
		tr := &languageTransport{}
		s, a := languageSession(t, tr)
		a.cfg.TrapChannelID = "channel"
		m := msg("m1", user, "hello")
		m.Author.Bot = user == "otherbot"
		a.onMessage(s, m)
		if len(tr.methods) != 2 || tr.methods[0] != "DELETE" || tr.methods[1] != "PUT" {
			t.Fatalf("Trap must delete and ban %s, even without slurs: %+v", user, tr)
		}
	}
	tr := &languageTransport{}
	s, a := languageSession(t, tr)
	a.cfg.TrapChannelID = "channel"
	m := msg("own", "bot", "Automated warning")
	m.Author.Bot = true
	a.onMessage(s, m)
	a.banForPolicy(s, m, "posting in the bot-trap channel")
	if len(tr.paths) != 0 {
		t.Fatal("This bot must never be caught by its own trap")
	}
}
func TestTrapWebhookDeletesWithoutBanningWebhookID(t *testing.T) {
	tr := &languageTransport{}
	s, a := languageSession(t, tr)
	a.cfg.TrapChannelID = "channel"
	m := msg("webhookmessage", "webhook-id", "Spam")
	m.WebhookID = "webhook-id"
	a.onMessage(s, m)
	if len(tr.methods) != 1 || tr.methods[0] != "DELETE" {
		t.Fatal("Webhooks should be removed, not banned as if they were members")
	}
}
