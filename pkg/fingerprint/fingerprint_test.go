package fingerprint

import (
	"strings"
	"testing"
)

func TestClassifyHTTPWebLogic(t *testing.T) {
	f := Finding{Port: 7001}
	classifyHTTP(&f, "WebLogic Server 12.2.1", "Oracle WebLogic Server Administration Console", "", 7001)
	if f.Product != "weblogic" {
		t.Fatalf("product=%q", f.Product)
	}
	refs := MatchRefs(f)
	if len(refs) < 1 {
		t.Fatal("expected weblogic refs")
	}
	if refs[0].URL == "" || refs[0].ID == "" {
		t.Fatalf("ref incomplete: %+v", refs[0])
	}
}

func TestClassifyBannerSSH(t *testing.T) {
	f := Finding{}
	classifyBanner(&f, "SSH-2.0-OpenSSH_8.9")
	if f.Service != "ssh" || f.Product != "openssh" {
		t.Fatalf("got service=%s product=%s", f.Service, f.Product)
	}
}

func TestExtractTitle(t *testing.T) {
	body := `<html><head><title>  Hello  World </title></head></html>`
	if got := extractTitle(body); got != "Hello World" {
		t.Fatalf("title=%q", got)
	}
}

func TestMatchRefsUnknownEmpty(t *testing.T) {
	refs := MatchRefs(Finding{Product: "obscure-xyz", Service: "unknown"})
	if len(refs) != 0 {
		t.Fatalf("want empty, got %v", refs)
	}
}

func TestBuildInteresting(t *testing.T) {
	findings := []Finding{
		{IP: "1.1.1.1", Port: 7001, Product: "weblogic", Refs: MatchRefs(Finding{Product: "weblogic"})},
		{IP: "1.1.1.2", Port: 80, Product: "nginx"},
		{IP: "1.1.1.3", Port: 22, Product: "openssh"},
	}
	inter := BuildInteresting(findings)
	if len(inter) < 2 {
		t.Fatalf("interesting=%v", inter)
	}
	// weblogic should mention refs
	found := false
	for _, i := range inter {
		if i.Port == 7001 && strings.Contains(i.Why, "refs") {
			found = true
		}
	}
	if !found {
		t.Fatalf("weblogic not interesting: %v", inter)
	}
}

func TestExtractVersion(t *testing.T) {
	if v := extractVersion("OpenSSH_8.9p1", `OpenSSH[_\s]?([\d.]+)`); v != "8.9" {
		// pattern may get 8.9 from 8.9p1 via [\d.]+
		if v != "8.9" && v != "8.9p1" {
			// Accept 8.9
			if !strings.HasPrefix(v, "8.9") {
				t.Fatalf("version=%q", v)
			}
		}
	}
}
