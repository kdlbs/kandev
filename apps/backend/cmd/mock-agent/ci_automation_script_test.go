package main

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestCIAutomationScriptKeepsRepairActiveUntilCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		updater := newCapturingUpdater()
		done := make(chan struct{})
		defer func() {
			cancel()
			<-done
		}()
		go func() {
			defer close(done)
			handlePrompt(&emitter{ctx: ctx, conn: updater, sid: "repair-session"},
				"@ci-auto-fix\nPR: acme/demo#144\n\n"+
					"<kandev-system>e2e:message(\"repair active\")\ne2e:delay(60000)</kandev-system>",
				modelFast)
		}()
		synctest.Wait()
		texts := updater.textMessages()
		if len(texts) != 1 || texts[0] != "repair active" {
			t.Fatalf("repair output = %v, want the scripted acceptance marker", texts)
		}
		select {
		case <-done:
			t.Fatal("repair completed before cancellation")
		default:
		}
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("repair script ignored cancellation")
		}
	})
}

func TestCIAutomationScriptScope(t *testing.T) {
	for _, prompt := range []string{
		"ordinary prompt\n<kandev-system>e2e:delay(60000)</kandev-system>",
		"@ci-auto-fix-other\n<kandev-system>e2e:delay(60000)</kandev-system>",
		"@ci-auto-fix\ne2e:delay(60000)",
		"@ci-auto-fix\n<kandev-system>Instructions mention e2e:delay(60000)</kandev-system>",
	} {
		if got := ciAutomationScript(prompt, stripKandevSystem(prompt)); got != "" {
			t.Fatalf("unexpected script %q for %q", got, prompt)
		}
	}
}
